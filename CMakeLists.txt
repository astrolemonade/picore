cmake_minimum_required(VERSION 3.18)

set(PICO_SDK_PATH "../pico-sdk")

include(pico_sdk_import.cmake)

project(picore C CXX ASM)

set(CMAKE_C_STANDARD 11)
set(CMAKE_CXX_STANDARD 17)
pico_sdk_init()

add_executable(picore main.c)

target_link_libraries(picore pico_stdlib hardware_pwm hardware_adc pico_multicore)

pico_enable_stdio_usb(picore 1)
pico_enable_stdio_uart(picore 0)
pico_add_extra_outputs(picore)



