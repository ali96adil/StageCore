#pragma once

#include <string>

#include "esp_err.h"

namespace stagecore {

struct HubBinding {
  std::string hub_id;
  std::string fingerprint;
  std::string tls_sha256;

  bool complete() const;
};

class FoundationStore {
 public:
  // Set this to the device's existing namespace during migration so trust and
  // Setup AP overrides survive the move to the shared component.
  explicit FoundationStore(const char *nvs_namespace);

  esp_err_t LoadSetupAPPasswordOverride(std::string *password) const;
  esp_err_t EffectiveSetupAPPassword(std::string *password) const;
  esp_err_t SaveSetupAPPasswordOverride(const std::string &password) const;
  esp_err_t ResetSetupAPPasswordToDefault() const;

  esp_err_t LoadHubBinding(HubBinding *binding) const;
  esp_err_t SaveHubBinding(const HubBinding &binding) const;
  esp_err_t ClearHubBinding() const;

 private:
  std::string nvs_namespace_;
};

}  // namespace stagecore
