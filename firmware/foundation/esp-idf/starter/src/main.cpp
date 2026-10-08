#include "device_identity.h"
#include "foundation_contract.h"
#include "foundation_store.h"
#include "hub_discovery.h"
#include "hub_security.h"
#include "nvs_flash.h"
#include "trusted_clock.h"

extern "C" void app_main(void) {
  ESP_ERROR_CHECK(nvs_flash_init());

  stagecore::FoundationStore store("stagecore_test");
  stagecore::DeviceIdentity identity;
  ESP_ERROR_CHECK(identity.LoadOrCreate());

  stagecore::FoundationDeviceDescriptor descriptor{
      .hostname_prefix = "stagecore-test-",
      .platform = "esp32",
#if CONFIG_IDF_TARGET_ESP32C3
      .architecture = "riscv32",
#else
      .architecture = "xtensa",
#endif
      .firmware_version = "foundation-ci",
      .capabilities = {stagecore::kSetupAPPasswordCapability},
  };

  // Compile/link smoke only. Network discovery/authentication is intentionally
  // not started by the starter image.
  (void)store;
  (void)descriptor;
}
