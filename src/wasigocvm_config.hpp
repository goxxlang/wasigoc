// wasigocvm feature bits — included early from runtime.hpp when
// WASIGO_GOCVM is set. Keeps product capability probes in one place.
#pragma once

#if defined(WASIGO_GOCVM) && WASIGO_GOCVM

#ifndef WASIGO_GOCVM_VERSION
#define WASIGO_GOCVM_VERSION 1
#endif

// Sockets: wasigocvm_net.hpp poll bridge (today). Threads later.
#define WASIGO_GOCVM_HAS_SOCKETS 1

#if defined(__has_include)
#  if __has_include(<pthread.h>) && defined(_REENTRANT)
#    define WASIGO_GOCVM_HAS_PTHREAD 1
#  endif
#endif
#ifndef WASIGO_GOCVM_HAS_PTHREAD
#define WASIGO_GOCVM_HAS_PTHREAD 0
#endif

#endif  // WASIGO_GOCVM
