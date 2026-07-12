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

if (EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/tests")
    add_executable(test_runtime tests/test_runtime.cpp)
    target_link_libraries(test_runtime PRIVATE helix_rt)
    if(WIN32)
        target_link_libraries(test_runtime PRIVATE ws2_32)
    endif()

    add_executable(test_optimization tests/test_optimization.cpp)
    target_link_libraries(test_optimization PRIVATE helix_rt)
    if(WIN32)
        target_link_libraries(test_optimization PRIVATE ws2_32)
    endif()
endif()
