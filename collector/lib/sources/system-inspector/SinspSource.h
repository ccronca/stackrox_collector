#ifndef _SINSP_SOURCE_H
#define _SINSP_SOURCE_H

#include <atomic>
#include <bitset>
#include <memory>
#include <mutex>
#include <string>

#include <gtest/gtest_prod.h>

#include "CollectorConfig.h"
#include "ContainerMetadata.h"
#include "Control.h"
#include "DriverCandidates.h"
#include "SignalHandler.h"
#include "SignalServiceClient.h"
#include "Source.h"

// forward declarations
class sinsp;
class sinsp_evt;
class sinsp_evt_formatter;
class sinsp_threadinfo;

namespace collector {
namespace sources {

class FalcoSource : public ISource {
 public:
  static constexpr int kMessageBufferSize = 8192;
  static constexpr int kKeyBufferSize = 48;

  FalcoSource();
  std::shared_ptr<Signal> Next() override;
  bool Init(const CollectorConfig& config) override;
  void Start() override;
  void Stop() override;

 private:
  FRIEND_TEST(SystemInspectorServiceTest, FilterEvent);

  struct SignalHandlerEntry {
    std::unique_ptr<SignalHandler> handler;
    std::bitset<PPM_EVENT_MAX> event_filter;

    SignalHandlerEntry(std::unique_ptr<SignalHandler> handler, std::bitset<PPM_EVENT_MAX> event_filter)
        : handler(std::move(handler)), event_filter(event_filter) {}

    bool ShouldHandle(sinsp_evt* evt) const;
  };

  sinsp_evt* GetNext();
  static bool FilterEvent(sinsp_evt* event);
  static bool FilterEvent(const sinsp_threadinfo* tinfo);

  bool SendExistingProcesses(SignalHandler* handler);

  void AddSignalHandler(std::unique_ptr<SignalHandler> signal_handler);

  mutable std::mutex libsinsp_mutex_;
  std::unique_ptr<sinsp> inspector_;
  std::shared_ptr<ContainerMetadata> container_metadata_inspector_;
  std::unique_ptr<sinsp_evt_formatter> default_formatter_;
  std::unique_ptr<ISignalServiceClient> signal_client_;
  std::vector<SignalHandlerEntry> signal_handlers_;
  Stats userspace_stats_;
  std::bitset<PPM_EVENT_MAX> global_event_filter_;

  mutable std::mutex running_mutex_;
  bool running_ = false;

  void ServePendingProcessRequests();
  mutable std::mutex process_requests_mutex_;
  // [ ( pid, callback ), ( pid, callback ), ... ]
  std::list<std::pair<uint64_t, ProcessInfoCallbackRef>> pending_process_requests_;
};

}  // namespace sources
}  // namespace collector

#endif
