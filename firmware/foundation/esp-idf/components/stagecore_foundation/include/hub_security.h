#pragma once

#include <string>
#include <vector>

#include "device_identity.h"
#include "esp_err.h"
#include "hub_discovery.h"

namespace stagecore {

struct FoundationDeviceDescriptor {
  std::string hostname_prefix;
  std::string platform;
  std::string architecture;
  std::string firmware_version;
  std::vector<std::string> capabilities;

  bool complete() const {
    return !hostname_prefix.empty() && !platform.empty() &&
           !architecture.empty() && !firmware_version.empty();
  }
};

struct RuntimeCredential {
  std::string session_id;
  std::string token;
};

esp_err_t ensure_paired_and_authenticate(
    const VerifiedHub &hub, DeviceIdentity *identity,
    const FoundationDeviceDescriptor &descriptor,
    const std::string &display_name, RuntimeCredential *credential);

}  // namespace stagecore
