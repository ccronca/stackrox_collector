
#ifndef COLLECTOR_SELF_CHECK_HANDLE_H
#define COLLECTOR_SELF_CHECK_HANDLE_H

#include <chrono>

#include "SelfChecks.h"
#include "SignalHandler.h"
#include "SysdigEventExtractor.h"

namespace collector {

class SelfCheckHandler : public SignalHandler {
 public:
  SelfCheckHandler() {}
  SelfCheckHandler(
      sinsp* inspector,
      std::chrono::seconds timeout = std::chrono::seconds(5)) : inspector_(inspector), timeout_(timeout) {
    event_extractor_.Init(inspector);
    start_ = std::chrono::steady_clock::now();
  }

  /**
   * @brief Verifies that a given event came from the self-check process,
   * by checking the process name and the executable path.
   *
   * @note pid verification is not possible because the driver retrieves
   *       the host pid, but when we fork the process we get the namespace
   *       pid.
   */
  static bool IsSelfCheckEvent(sinsp_evt* evt, SysdigEventExtractor& event_extractor) {
    const std::string* name = event_extractor.get_comm(evt);
    const std::string* exe = event_extractor.get_exe(evt);

    if (name == nullptr || exe == nullptr) {
      return false;
    }

    return IsSelfCheckEvent(*name, *exe);
  }

  static bool IsSelfCheckEvent(sinsp_threadinfo& tinfo) {
    const std::string& name = tinfo.get_comm();
    const std::string& exe = tinfo.get_exepath();

    return IsSelfCheckEvent(name, exe);
  }

  static bool IsSelfCheckEvent(const std::string& name, const std::string& exe) {
    return name.compare(self_checks::kSelfChecksName) == 0 || exe.compare(self_checks::kSelfChecksExePath);
  }

  /**
   * @brief simple check that the handler has timed out waiting for
   *        self check events.
   */
  bool hasTimedOut() {
    auto now = std::chrono::steady_clock::now();
    return now > (start_ + timeout_);
  }

 protected:
  sinsp* inspector_;
  SysdigEventExtractor event_extractor_;

  std::chrono::time_point<std::chrono::steady_clock> start_;
  std::chrono::seconds timeout_;

  bool seen_self_check_ = false;
};

class SelfCheckProcessHandler : public SelfCheckHandler {
 public:
  SelfCheckProcessHandler(sinsp* inspector) : SelfCheckHandler(inspector) {
  }

  std::string GetName() override {
    return "SelfCheckProcessHandler";
  }

  std::vector<std::string> GetRelevantEvents() {
    return {"execve<"};
  }

  virtual Result HandleSignal(sinsp_evt* evt) override;
};

class SelfCheckNetworkHandler : public SelfCheckHandler {
 public:
  SelfCheckNetworkHandler(sinsp* inspector) : SelfCheckHandler(inspector) {
  }

  std::string GetName() override {
    return "SelfCheckNetworkHandler";
  }

  std::vector<std::string> GetRelevantEvents() {
    return {
        "close<",
        "shutdown<",
        "connect<",
        "accept<",
        "getsockopt<",
    };
  }

  Result HandleSignal(sinsp_evt* evt) override;
};

}  // namespace collector

#endif
