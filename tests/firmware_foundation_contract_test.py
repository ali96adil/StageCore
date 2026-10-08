import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
COMP = ROOT / "firmware/foundation/esp-idf/components/stagecore_foundation"


class FirmwareFoundationContractTest(unittest.TestCase):
    def read(self, relative):
        return (COMP / relative).read_text(encoding="utf-8")

    def test_required_component_surface_exists(self):
        required = [
            "CMakeLists.txt",
            "idf_component.yml",
            "include/device_identity.h",
            "include/foundation_contract.h",
            "include/foundation_store.h",
            "include/hub_discovery.h",
            "include/hub_security.h",
            "include/trusted_clock.h",
            "src/device_identity.cpp",
            "src/foundation_store.cpp",
            "src/hub_discovery.cpp",
            "src/hub_security.cpp",
            "src/trusted_clock.cpp",
        ]
        for path in required:
            self.assertTrue((COMP / path).is_file(), path)

    def test_setup_ap_policy_is_canonical(self):
        contract = self.read("include/foundation_contract.h")
        store = self.read("src/foundation_store.cpp")
        self.assertIn('kDefaultSetupAPPassword[] = "12345678"', contract)
        self.assertIn('"device.maintenance.setup-ap-password"', contract)
        self.assertIn("length >= 8 && length <= 63", contract)
        self.assertIn('"setup_ap_pass"', store)
        self.assertIn("EffectiveSetupAPPassword", store)
        self.assertIn("ResetSetupAPPasswordToDefault", store)

    def test_hub_trust_is_store_backed_and_fail_closed(self):
        header = self.read("include/hub_discovery.h")
        source = self.read("src/hub_discovery.cpp")
        store = self.read("src/foundation_store.cpp")
        self.assertIn("FoundationStore *store", header)
        self.assertIn("store->LoadHubBinding", source)
        self.assertIn("store->SaveHubBinding", source)
        self.assertIn("present != 3 || !loaded.complete()", store)
        self.assertIn("TLS pin mismatch", source)

    def test_device_specific_output_authority_is_not_baked_in(self):
        text = "\n".join(
            p.read_text(encoding="utf-8")
            for p in COMP.rglob("*")
            if p.is_file() and p.suffix in {".h", ".cpp"}
        )
        for forbidden in (
            "lighting.channels.",
            "lighting.blackout",
            "laser.arm",
            "laser.state.",
            "camera.flash",
            "dmx.",
        ):
            self.assertNotIn(forbidden, text)

    def test_security_uses_consumer_descriptor(self):
        header = self.read("include/hub_security.h")
        source = self.read("src/hub_security.cpp")
        self.assertIn("FoundationDeviceDescriptor", header)
        self.assertIn("descriptor.hostname_prefix", source)
        self.assertIn("descriptor.capabilities", source)
        self.assertNotIn("stagecore-light-", source)
        self.assertNotIn("STAGECORE_FW_VERSION", source)


if __name__ == "__main__":
    unittest.main()
