/*
 * Copyright (C) 2022 The Falco Authors.
 *
 * This file is dual licensed under either the MIT or GPL 2. See MIT.txt
 * or GPL2.txt for full copies of the license.
 */

#include <helpers/interfaces/fixed_size_event.h>
#include <helpers/interfaces/variable_size_event.h>

/*=============================== ENTER EVENT ===========================*/

SEC("ksyscall/getsockopt")
int BPF_KSYSCALL(sys_enter_getsockopt) {
  struct ringbuf_struct ringbuf;
  if (!ringbuf__reserve_space(&ringbuf, GETSOCKOPT_E_SIZE)) {
    return 0;
  }

  ringbuf__store_event_header(&ringbuf, PPME_SOCKET_GETSOCKOPT_E);

  /*=============================== COLLECT PARAMETERS  ===========================*/

  // Here we have no parameters to collect.

  /*=============================== COLLECT PARAMETERS  ===========================*/

  ringbuf__submit_event(&ringbuf);

  return 0;
}

/*=============================== ENTER EVENT ===========================*/

/*=============================== EXIT EVENT ===========================*/

SEC("kretsyscall/getsockopt")
int BPF_KSYSCALL(sys_exit_getsockopt, long ret) {
  struct auxiliary_map* auxmap = auxmap__get();
  if (!auxmap) {
    return 0;
  }

  auxmap__preload_event_header(auxmap, PPME_SOCKET_GETSOCKOPT_X);

  /*=============================== COLLECT PARAMETERS  ===========================*/

  /* Collect parameters at the beginning to manage socketcalls */
  unsigned long args[5];
  extract__network_args(args, 5, ctx);

  /* Parameter 1: res (type: PT_ERRNO) */
  auxmap__store_s64_param(auxmap, ret);

  /* Parameter 2: fd (type: PT_FD) */
  s32 fd = (s32)args[0];
  auxmap__store_s64_param(auxmap, (s64)fd);

  /* Parameter 3: level (type: PT_ENUMFLAGS8) */
  int level = args[1];
  auxmap__store_u8_param(auxmap, sockopt_level_to_scap(level));

  /* Parameter 4: optname (type: PT_ENUMFLAGS8) */
  int optname = args[2];
  auxmap__store_u8_param(auxmap, sockopt_optname_to_scap(level, optname));

  /* `optval` and `optlen` will be the ones provided by the user if the syscall fails
   * otherwise they will refer to the real socket data since the kernel populated them.
   */

  /* Parameter 5: optval (type: PT_DYN) */
  unsigned long optval = args[3];
  int optlen = 0;
  unsigned long optlen_pointer = args[4];
  bpf_probe_read_user(&optlen, sizeof(optlen), (void*)optlen_pointer);
  auxmap__store_sockopt_param(auxmap, level, optname, optlen, optval);

  /* Parameter 6: optlen (type: PT_UINT32) */
  auxmap__store_u32_param(auxmap, (u32)optlen);

  /*=============================== COLLECT PARAMETERS  ===========================*/

  auxmap__finalize_event_header(auxmap);

  auxmap__submit_event(auxmap);

  return 0;
}

/*=============================== EXIT EVENT ===========================*/
