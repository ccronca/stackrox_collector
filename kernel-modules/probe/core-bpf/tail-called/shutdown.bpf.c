/*
 * Copyright (C) 2022 The Falco Authors.
 *
 * This file is dual licensed under either the MIT or GPL 2. See MIT.txt
 * or GPL2.txt for full copies of the license.
 */
#include <preamble.h>

#include <helpers/interfaces/fixed_size_event.h>

/*=============================== ENTER EVENT ===========================*/

SEC("ksyscall/shutdown")
int BPF_KSYSCALL(sys_enter_shutdown) {
  if (!preamble(__NR_shutdown)) {
    return 0;
  }

  struct ringbuf_struct ringbuf;
  if (!ringbuf__reserve_space(&ringbuf, SHUTDOWN_E_SIZE)) {
    return 0;
  }

  ringbuf__store_event_header(&ringbuf, PPME_SOCKET_SHUTDOWN_E);

  /*=============================== COLLECT PARAMETERS  ===========================*/

  /* Collect parameters at the beginning to easily manage socketcalls */
  unsigned long args[2];
  extract__network_args(args, 2, ctx);

  /* Parameter 1: fd (type: PT_FD) */
  s32 fd = (s32)args[0];
  ringbuf__store_s64(&ringbuf, (s64)fd);

  /* Parameter 2: how (type: PT_ENUMFLAGS8) */
  int how = (s32)args[1];
  ringbuf__store_u8(&ringbuf, (u8)shutdown_how_to_scap(how));

  /*=============================== COLLECT PARAMETERS  ===========================*/

  ringbuf__submit_event(&ringbuf);

  return 0;
}

/*=============================== ENTER EVENT ===========================*/

/*=============================== EXIT EVENT ===========================*/

SEC("kretsyscall/shutdown")
int BPF_KSYSCALL(sys_exit_shutdown, long ret) {
  if (!preamble(__NR_shutdown)) {
    return 0;
  }

  struct ringbuf_struct ringbuf;
  if (!ringbuf__reserve_space(&ringbuf, SHUTDOWN_X_SIZE)) {
    return 0;
  }

  ringbuf__store_event_header(&ringbuf, PPME_SOCKET_SHUTDOWN_X);

  /*=============================== COLLECT PARAMETERS  ===========================*/

  /* Parameter 1: res (type: PT_ERRNO)*/
  ringbuf__store_s64(&ringbuf, ret);

  /*=============================== COLLECT PARAMETERS  ===========================*/

  ringbuf__submit_event(&ringbuf);

  return 0;
}

/*=============================== EXIT EVENT ===========================*/
