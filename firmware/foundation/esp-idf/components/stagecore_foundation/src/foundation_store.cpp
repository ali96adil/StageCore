#include "foundation_store.h"

#include <utility>
#include <vector>

#include "foundation_contract.h"
#include "nvs.h"

namespace stagecore {
namespace {

constexpr char kHubIDKey[] = "hub_id";
constexpr char kHubFingerprintKey[] = "hub_fp";
constexpr char kHubTLSKey[] = "hub_tls";
constexpr char kSetupAPPasswordKey[] = "setup_ap_pass";

esp_err_t read_string(nvs_handle_t handle, const char *key, std::string *value,
                      bool *found = nullptr) {
  if (value == nullptr) return ESP_ERR_INVALID_ARG;
  if (found != nullptr) *found = false;

  size_t length = 0;
  esp_err_t err = nvs_get_str(handle, key, nullptr, &length);
  if (err == ESP_ERR_NVS_NOT_FOUND) {
    value->clear();
    return ESP_OK;
  }
  if (err != ESP_OK) return err;

  if (found != nullptr) *found = true;
  if (length == 0) {
    value->clear();
    return ESP_OK;
  }

  std::vector<char> buffer(length);
  err = nvs_get_str(handle, key, buffer.data(), &length);
  if (err == ESP_OK) *value = buffer.data();
  return err;
}

esp_err_t erase_key_if_present(nvs_handle_t handle, const char *key) {
  const esp_err_t err = nvs_erase_key(handle, key);
  return err == ESP_ERR_NVS_NOT_FOUND ? ESP_OK : err;
}

}  // namespace

bool HubBinding::complete() const {
  return hub_id.size() == 36 && !fingerprint.empty() && tls_sha256.size() == 64;
}

FoundationStore::FoundationStore(const char *nvs_namespace)
    : nvs_namespace_(nvs_namespace != nullptr ? nvs_namespace : "") {}

esp_err_t FoundationStore::LoadSetupAPPasswordOverride(
    std::string *password) const {
  if (password == nullptr || nvs_namespace_.empty()) return ESP_ERR_INVALID_ARG;

  nvs_handle_t handle;
  esp_err_t err = nvs_open(nvs_namespace_.c_str(), NVS_READONLY, &handle);
  if (err == ESP_ERR_NVS_NOT_FOUND) {
    password->clear();
    return ESP_OK;
  }
  if (err != ESP_OK) return err;

  err = read_string(handle, kSetupAPPasswordKey, password);
  nvs_close(handle);
  if (err != ESP_OK) return err;

  if (!password->empty() &&
      !valid_setup_ap_password_length(password->size())) {
    password->clear();
    return ESP_ERR_INVALID_STATE;
  }
  return ESP_OK;
}

esp_err_t FoundationStore::EffectiveSetupAPPassword(
    std::string *password) const {
  esp_err_t err = LoadSetupAPPasswordOverride(password);
  if (err != ESP_OK) return err;
  if (password->empty()) *password = kDefaultSetupAPPassword;
  return ESP_OK;
}

esp_err_t FoundationStore::SaveSetupAPPasswordOverride(
    const std::string &password) const {
  if (nvs_namespace_.empty() ||
      !valid_setup_ap_password_length(password.size())) {
    return ESP_ERR_INVALID_ARG;
  }

  nvs_handle_t handle;
  esp_err_t err = nvs_open(nvs_namespace_.c_str(), NVS_READWRITE, &handle);
  if (err != ESP_OK) return err;
  err = nvs_set_str(handle, kSetupAPPasswordKey, password.c_str());
  if (err == ESP_OK) err = nvs_commit(handle);
  nvs_close(handle);
  return err;
}

esp_err_t FoundationStore::ResetSetupAPPasswordToDefault() const {
  if (nvs_namespace_.empty()) return ESP_ERR_INVALID_ARG;

  nvs_handle_t handle;
  esp_err_t err = nvs_open(nvs_namespace_.c_str(), NVS_READWRITE, &handle);
  if (err == ESP_ERR_NVS_NOT_FOUND) return ESP_OK;
  if (err != ESP_OK) return err;
  err = erase_key_if_present(handle, kSetupAPPasswordKey);
  if (err == ESP_OK) err = nvs_commit(handle);
  nvs_close(handle);
  return err;
}

esp_err_t FoundationStore::LoadHubBinding(HubBinding *binding) const {
  if (binding == nullptr || nvs_namespace_.empty()) return ESP_ERR_INVALID_ARG;

  nvs_handle_t handle;
  esp_err_t err = nvs_open(nvs_namespace_.c_str(), NVS_READONLY, &handle);
  if (err == ESP_ERR_NVS_NOT_FOUND) {
    *binding = HubBinding{};
    return ESP_OK;
  }
  if (err != ESP_OK) return err;

  HubBinding loaded;
  bool id_found = false;
  bool fingerprint_found = false;
  bool tls_found = false;

  err = read_string(handle, kHubIDKey, &loaded.hub_id, &id_found);
  if (err == ESP_OK) {
    err = read_string(handle, kHubFingerprintKey, &loaded.fingerprint,
                      &fingerprint_found);
  }
  if (err == ESP_OK) {
    err = read_string(handle, kHubTLSKey, &loaded.tls_sha256, &tls_found);
  }
  nvs_close(handle);
  if (err != ESP_OK) return err;

  const int present = static_cast<int>(id_found) +
                      static_cast<int>(fingerprint_found) +
                      static_cast<int>(tls_found);
  if (present == 0) {
    *binding = HubBinding{};
    return ESP_OK;
  }
  if (present != 3 || !loaded.complete()) return ESP_ERR_INVALID_STATE;

  *binding = std::move(loaded);
  return ESP_OK;
}

esp_err_t FoundationStore::SaveHubBinding(const HubBinding &binding) const {
  if (nvs_namespace_.empty() || !binding.complete()) return ESP_ERR_INVALID_ARG;

  nvs_handle_t handle;
  esp_err_t err = nvs_open(nvs_namespace_.c_str(), NVS_READWRITE, &handle);
  if (err != ESP_OK) return err;

  err = nvs_set_str(handle, kHubIDKey, binding.hub_id.c_str());
  if (err == ESP_OK) {
    err = nvs_set_str(handle, kHubFingerprintKey, binding.fingerprint.c_str());
  }
  if (err == ESP_OK) {
    err = nvs_set_str(handle, kHubTLSKey, binding.tls_sha256.c_str());
  }
  if (err == ESP_OK) err = nvs_commit(handle);
  nvs_close(handle);
  return err;
}

esp_err_t FoundationStore::ClearHubBinding() const {
  if (nvs_namespace_.empty()) return ESP_ERR_INVALID_ARG;

  nvs_handle_t handle;
  esp_err_t err = nvs_open(nvs_namespace_.c_str(), NVS_READWRITE, &handle);
  if (err == ESP_ERR_NVS_NOT_FOUND) return ESP_OK;
  if (err != ESP_OK) return err;

  err = erase_key_if_present(handle, kHubIDKey);
  if (err == ESP_OK) err = erase_key_if_present(handle, kHubFingerprintKey);
  if (err == ESP_OK) err = erase_key_if_present(handle, kHubTLSKey);
  if (err == ESP_OK) err = nvs_commit(handle);
  nvs_close(handle);
  return err;
}

}  // namespace stagecore
