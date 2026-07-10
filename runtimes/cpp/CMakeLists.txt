cmake_minimum_required(VERSION 3.14)
project(HelixRuntimeCpp)

set(CMAKE_CXX_STANDARD 17)

# We assume hiredis, redis++, and libmemcached are available in the environment
find_path(HIREDIS_INCLUDE_DIR hiredis/hiredis.h)
find_path(REDIS_PLUS_PLUS_INCLUDE_DIR sw/redis++/redis++.h)
find_path(MEMCACHED_INCLUDE_DIR libmemcached/memcached.h)

include_directories(include)
if (HIREDIS_INCLUDE_DIR)
    include_directories(${HIREDIS_INCLUDE_DIR})
endif()
if (REDIS_PLUS_PLUS_INCLUDE_DIR)
    include_directories(${REDIS_PLUS_PLUS_INCLUDE_DIR})
endif()
if (MEMCACHED_INCLUDE_DIR)
    include_directories(${MEMCACHED_INCLUDE_DIR})
endif()

add_library(helix_rt INTERFACE)
target_include_directories(helix_rt INTERFACE include)
