#pragma once

#include <cstddef>

namespace stagecore {

inline constexpr char kFoundationVersion[] = "1";
inline constexpr char kDefaultSetupAPPassword[] = "12345678";
inline constexpr char kSetupAPPasswordCapability[] =
    "device.maintenance.setup-ap-password";

inline constexpr bool valid_setup_ap_password_length(std::size_t length) {
  return length >= 8 && length <= 63;
}

}  // namespace stagecore
