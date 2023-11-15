/*
 * Copyright (C) 2022 The Falco Authors.
 *
 * This file is dual licensed under either the MIT or GPL 2. See MIT.txt
 * or GPL2.txt for full copies of the license.
 */

#include <asm-generic/errno.h>
#include <helpers/interfaces/variable_size_event.h>

/*=============================== ENTER EVENT ===========================*/

SEC("ksyscall/connect")
int BPF_KSYSCALL(sys_enter_connect) {
  struct auxiliary_map* auxmap = auxmap__get();
  if (!auxmap) {
    return 0;
  }
  auxmap__preload_event_header(auxmap, PPME_SOCKET_CONNECT_E);

  /*=============================== COLLECT PARAMETERS  ===========================*/

  unsigned long args[3];
  extract__network_args(args, 3, ctx);

  /* Parameter 1: fd (type: PT_FD)*/
  s32 socket_fd = (s32)args[0];
  auxmap__store_s64_param(auxmap, (s64)socket_fd);

  /* Parameter 2: addr (type: PT_SOCKADDR)*/
  unsigned long sockaddr_ptr = args[1];
  u16 addrlen = (u16)args[2];
  auxmap__store_sockaddr_param(auxmap, sockaddr_ptr, addrlen);

  /*=============================== COLLECT PARAMETERS  ===========================*/

  auxmap__finalize_event_header(auxmap);

  auxmap__submit_event(auxmap);

  return 0;
}

/*=============================== ENTER EVENT ===========================*/

/*=============================== EXIT EVENT ===========================*/

SEC("kretsyscall/connect")
int BPF_KSYSCALL(sys_exit_connect, long ret) {
  struct auxiliary_map* auxmap = auxmap__get();
  if (!auxmap) {
    return 0;
  }

  auxmap__preload_event_header(auxmap, PPME_SOCKET_CONNECT_X);

  /*=============================== COLLECT PARAMETERS  ===========================*/

  unsigned long args[1];
  extract__network_args(args, 1, ctx);

  /* Parameter 1: res (type: PT_ERRNO) */
  auxmap__store_s64_param(auxmap, ret);

  s32 socket_fd = (s32)args[0];

  /* Parameter 2: tuple (type: PT_SOCKTUPLE) */
  /* We need a valid sockfd to extract source data.*/
  if (ret == 0 || ret == -EINPROGRESS) {
    auxmap__store_socktuple_param(auxmap, socket_fd, OUTBOUND);
  } else {
    auxmap__store_empty_param(auxmap);
  }

  /*=============================== COLLECT PARAMETERS  ===========================*/

  auxmap__finalize_event_header(auxmap);

  auxmap__submit_event(auxmap);

  return 0;
}

/*=============================== EXIT EVENT ===========================*/
