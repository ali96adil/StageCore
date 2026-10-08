idf_component_register(
    SRCS
        "src/device_identity.cpp"
        "src/foundation_store.cpp"
        "src/hub_discovery.cpp"
        "src/hub_security.cpp"
        "src/trusted_clock.cpp"
    INCLUDE_DIRS "include"
    REQUIRES
        esp_http_client
        esp-tls
        esp_netif
        json
        mbedtls
        mdns
        nvs_flash
)
