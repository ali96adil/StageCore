#include "hub_discovery.h"

#include <array>
#include <cerrno>
#include <cctype>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <strings.h>
#include <string>
#include <vector>

#include "cJSON.h"
#include "foundation_store.h"
#include "esp_http_client.h"
#include "esp_log.h"
#include "esp_netif.h"
#include "esp_tls.h"
#include "lwip/ip_addr.h"
#include "lwip/sockets.h"
#include "mbedtls/ctr_drbg.h"
#include "mbedtls/entropy.h"
#include "mbedtls/ssl.h"
#include "sha/sha_core.h"
#include "mdns.h"
#include "trusted_clock.h"

namespace stagecore {
namespace {

constexpr char kTag[] = "stagecore-hub";
constexpr char kService[] = "_stagecore-hub";
constexpr char kProto[] = "_tcp";
constexpr uint32_t kDiscoveryTimeoutMS = 2500;
constexpr size_t kMaxDiscoveryResults = 8;

struct Candidate {
  std::string hub_id;
  std::string display_name;
  std::string fingerprint;
  std::string tls_sha256;
  std::string host;
  std::string address;
  uint16_t port = 0;
};

struct HttpBody {
  std::string body;
  std::string date_header;
};

bool valid_uuid_shape(const std::string &value) {
  if (value.size() != 36) return false;
  for (size_t i = 0; i < value.size(); ++i) {
    const bool dash = i == 8 || i == 13 || i == 18 || i == 23;
    if (dash) {
      if (value[i] != '-') return false;
    } else if (!std::isxdigit(static_cast<unsigned char>(value[i]))) {
      return false;
    }
  }
  return true;
}

bool valid_hex_sha256(const std::string &value) {
  if (value.size() != 64) return false;
  for (char ch : value) {
    if (!std::isxdigit(static_cast<unsigned char>(ch))) return false;
  }
  return true;
}

std::string lower_ascii(std::string value) {
  for (char &ch : value) ch = static_cast<char>(std::tolower(static_cast<unsigned char>(ch)));
  return value;
}

std::string txt_value(const mdns_result_t *result, const char *key) {
  if (result == nullptr || key == nullptr) return {};
  for (size_t i = 0; i < result->txt_count; ++i) {
    if (result->txt[i].key == nullptr ||
        std::strcmp(result->txt[i].key, key) != 0 ||
        result->txt[i].value == nullptr) {
      continue;
    }
    if (result->txt_value_len != nullptr) {
      return std::string(result->txt[i].value, result->txt_value_len[i]);
    }
    return result->txt[i].value;
  }
  return {};
}

bool extract_ipv4(const mdns_result_t *result, std::string *address) {
  if (result == nullptr || address == nullptr) return false;
  for (const mdns_ip_addr_t *item = result->addr; item != nullptr; item = item->next) {
    if (!IP_IS_V4(&item->addr)) continue;
    char text[IPADDR_STRLEN_MAX] = {};
    if (esp_ip4addr_ntoa(&item->addr.u_addr.ip4, text, sizeof(text)) != nullptr) {
      *address = text;
      return true;
    }
  }
  return false;
}

bool parse_candidate(const mdns_result_t *result, Candidate *candidate) {
  if (result == nullptr || candidate == nullptr || result->port == 0) return false;

  Candidate parsed;
  parsed.hub_id = lower_ascii(txt_value(result, "hub_id"));
  parsed.display_name = txt_value(result, "name");
  parsed.fingerprint = txt_value(result, "hub_fp");
  parsed.tls_sha256 = lower_ascii(txt_value(result, "tls_sha256"));
  parsed.host = lower_ascii(txt_value(result, "host"));

  const std::string version = txt_value(result, "v");
  const std::string port_text = txt_value(result, "port");
  const std::string api_path = txt_value(result, "api_path");
  const std::string runtime_path = txt_value(result, "runtime_path");

  char *end = nullptr;
  const long advertised_port = std::strtol(port_text.c_str(), &end, 10);
  if (version != "1" || !valid_uuid_shape(parsed.hub_id) ||
      parsed.display_name.empty() || parsed.display_name.size() > 96 ||
      parsed.fingerprint.empty() || parsed.fingerprint.size() > 200 ||
      !valid_hex_sha256(parsed.tls_sha256) ||
      parsed.host.size() < 7 || parsed.host.size() > 253 ||
      parsed.host.substr(parsed.host.size() - 6) != ".local" ||
      port_text.empty() || end == nullptr || *end != '\0' ||
      advertised_port <= 0 || advertised_port > 65535 ||
      static_cast<uint16_t>(advertised_port) != result->port ||
      api_path != "/" ||
      runtime_path != "/api/v1/companion/runtime" ||
      !extract_ipv4(result, &parsed.address)) {
    return false;
  }

  parsed.port = result->port;
  *candidate = std::move(parsed);
  return true;
}

bool binding_matches(const HubBinding &binding, const Candidate &candidate) {
  return binding.complete() &&
         lower_ascii(binding.hub_id) == candidate.hub_id &&
         binding.fingerprint == candidate.fingerprint &&
         lower_ascii(binding.tls_sha256) == candidate.tls_sha256;
}

std::string hex_digest(const unsigned char digest[32]) {
  static constexpr char kHex[] = "0123456789abcdef";
  std::string out(64, '0');
  for (size_t i = 0; i < 32; ++i) {
    out[i * 2] = kHex[(digest[i] >> 4) & 0x0f];
    out[i * 2 + 1] = kHex[digest[i] & 0x0f];
  }
  return out;
}

// This bootstrap does not send HTTP or other application data. The Gateway uses
// a self-signed certificate, so the leaf is accepted *only* long enough to
// calculate and compare its SHA-256 against the expected discovery/binding pin.
// All subsequent HTTP/WebSocket traffic uses the pinned certificate as its CA.
// A first-time mDNS advertisement is not a cryptographic trust anchor; enrollment
// requires the operator to authenticate/approve the Hub identity separately.
int certificate_socket_send(void *ctx, const unsigned char *buf, size_t len) {
  return static_cast<int>(send(*static_cast<int *>(ctx), buf, len, 0));
}

int certificate_socket_recv(void *ctx, unsigned char *buf, size_t len) {
  const int n = static_cast<int>(recv(*static_cast<int *>(ctx), buf, len, 0));
  if (n < 0 && (errno == EAGAIN || errno == EWOULDBLOCK)) {
    return MBEDTLS_ERR_SSL_TIMEOUT;
  }
  return n;
}

esp_err_t capture_pinned_certificate(const Candidate &candidate,
                                     std::vector<unsigned char> *der) {
  if (der == nullptr || !valid_hex_sha256(candidate.tls_sha256)) {
    return ESP_ERR_INVALID_ARG;
  }
  der->clear();

  int fd = socket(AF_INET, SOCK_STREAM, IPPROTO_TCP);
  if (fd < 0) return ESP_FAIL;
  sockaddr_in address{};
  address.sin_family = AF_INET;
  address.sin_port = htons(candidate.port);
  if (inet_pton(AF_INET, candidate.address.c_str(), &address.sin_addr) != 1) {
    close(fd);
    return ESP_ERR_INVALID_ARG;
  }
  timeval timeout{};
  timeout.tv_sec = 5;
  setsockopt(fd, SOL_SOCKET, SO_RCVTIMEO, &timeout, sizeof(timeout));
  setsockopt(fd, SOL_SOCKET, SO_SNDTIMEO, &timeout, sizeof(timeout));

  mbedtls_entropy_context entropy;
  mbedtls_ctr_drbg_context rng;
  mbedtls_ssl_config conf;
  mbedtls_ssl_context ssl;
  mbedtls_entropy_init(&entropy);
  mbedtls_ctr_drbg_init(&rng);
  mbedtls_ssl_config_init(&conf);
  mbedtls_ssl_init(&ssl);

  esp_err_t outcome = ESP_FAIL;
  const char *personalization = "stagecore-hub-pin-bootstrap";
  int rc = mbedtls_ctr_drbg_seed(
      &rng, mbedtls_entropy_func, &entropy,
      reinterpret_cast<const unsigned char *>(personalization),
      std::strlen(personalization));
  if (rc != 0) goto cleanup;
  rc = mbedtls_ssl_config_defaults(
      &conf, MBEDTLS_SSL_IS_CLIENT, MBEDTLS_SSL_TRANSPORT_STREAM,
      MBEDTLS_SSL_PRESET_DEFAULT);
  if (rc != 0) goto cleanup;

  // Verification is intentionally deferred *only* for this handshake so the
  // self-signed leaf can be compared with the expected exact certificate pin.
  // Do not transmit any application data before this check succeeds.
  mbedtls_ssl_conf_authmode(&conf, MBEDTLS_SSL_VERIFY_OPTIONAL);
  mbedtls_ssl_conf_rng(&conf, mbedtls_ctr_drbg_random, &rng);
  rc = mbedtls_ssl_setup(&ssl, &conf);
  if (rc != 0) goto cleanup;
  rc = mbedtls_ssl_set_hostname(&ssl, candidate.host.c_str());
  if (rc != 0) goto cleanup;
  mbedtls_ssl_set_bio(&ssl, &fd, certificate_socket_send,
                      certificate_socket_recv, nullptr);

  if (connect(fd, reinterpret_cast<const sockaddr *>(&address),
              sizeof(address)) != 0) {
    goto cleanup;
  }
  do {
    rc = mbedtls_ssl_handshake(&ssl);
  } while (rc == MBEDTLS_ERR_SSL_WANT_READ ||
           rc == MBEDTLS_ERR_SSL_WANT_WRITE);
  if (rc != 0) goto cleanup;

  {
    const mbedtls_x509_crt *peer = mbedtls_ssl_get_peer_cert(&ssl);
    if (peer == nullptr || peer->raw.p == nullptr || peer->raw.len == 0) {
      goto cleanup;
    }
    unsigned char digest[32] = {};
    esp_sha(SHA2_256, peer->raw.p, peer->raw.len, digest);
    if (hex_digest(digest) != candidate.tls_sha256) {
      ESP_LOGE(kTag, "TLS pin mismatch for discovered Hub");
      outcome = ESP_ERR_INVALID_CRC;
      goto cleanup;
    }
    der->assign(peer->raw.p, peer->raw.p + peer->raw.len);
    outcome = ESP_OK;
  }

cleanup:
  if (outcome == ESP_OK) {
    ESP_LOGI(kTag, "TLS certificate SHA-256 pin verified");
  } else if (outcome != ESP_ERR_INVALID_CRC) {
    ESP_LOGE(kTag, "TLS preflight failed for %s:%u (mbedtls=%d)",
             candidate.address.c_str(), candidate.port, rc);
  }
  mbedtls_ssl_free(&ssl);
  mbedtls_ssl_config_free(&conf);
  mbedtls_ctr_drbg_free(&rng);
  mbedtls_entropy_free(&entropy);
  close(fd);
  return outcome;
}

esp_err_t http_event(esp_http_client_event_t *event) {
  if (event == nullptr || event->user_data == nullptr) return ESP_OK;
  auto *body = static_cast<HttpBody *>(event->user_data);
  if (event->event_id == HTTP_EVENT_ON_HEADER &&
      event->header_key != nullptr && event->header_value != nullptr &&
      strcasecmp(event->header_key, "Date") == 0) {
    body->date_header = event->header_value;
  } else if (event->event_id == HTTP_EVENT_ON_DATA &&
             event->data != nullptr && event->data_len > 0) {
    if (body->body.size() + static_cast<size_t>(event->data_len) > 8192) {
      return ESP_ERR_NO_MEM;
    }
    body->body.append(static_cast<const char *>(event->data),
                      static_cast<size_t>(event->data_len));
  }
  return ESP_OK;
}

esp_err_t verify_public_identity(const Candidate &candidate,
                                 const std::vector<unsigned char> &certificate,
                                 std::string *verified_name) {
  if (certificate.empty()) return ESP_ERR_INVALID_ARG;

  char url[160];
  std::snprintf(url, sizeof(url), "https://%s:%u/api/v1/hub/identity",
                candidate.address.c_str(), candidate.port);

  HttpBody body;
  esp_http_client_config_t config = {};
  config.url = url;
  config.method = HTTP_METHOD_GET;
  config.timeout_ms = 5000;
  config.cert_der = reinterpret_cast<const char *>(certificate.data());
  config.cert_len = certificate.size();
  config.skip_cert_common_name_check = true;
  config.tls_version = ESP_HTTP_CLIENT_TLS_VER_TLS_1_3;
  config.event_handler = http_event;
  config.user_data = &body;

  esp_http_client_handle_t client = esp_http_client_init(&config);
  if (client == nullptr) return ESP_ERR_NO_MEM;

  const esp_err_t performed = esp_http_client_perform(client);
  const int status = esp_http_client_get_status_code(client);

  esp_http_client_cleanup(client);
  if (performed != ESP_OK || status != 200) {
    ESP_LOGE(kTag, "Hub identity probe failed status=%d err=%s",
             status, esp_err_to_name(performed));
    return ESP_FAIL;
  }

  cJSON *root = cJSON_ParseWithLength(body.body.data(), body.body.size());
  if (root == nullptr) return ESP_ERR_INVALID_RESPONSE;

  const cJSON *schema = cJSON_GetObjectItemCaseSensitive(root, "schema_version");
  const cJSON *hub_id = cJSON_GetObjectItemCaseSensitive(root, "hub_id");
  const cJSON *display_name = cJSON_GetObjectItemCaseSensitive(root, "display_name");
  const cJSON *fingerprint = cJSON_GetObjectItemCaseSensitive(root, "fingerprint");

  const bool ok =
      cJSON_IsNumber(schema) && schema->valueint == 1 &&
      cJSON_IsString(hub_id) && hub_id->valuestring != nullptr &&
      lower_ascii(hub_id->valuestring) == candidate.hub_id &&
      cJSON_IsString(display_name) && display_name->valuestring != nullptr &&
      std::strlen(display_name->valuestring) > 0 &&
      cJSON_IsString(fingerprint) && fingerprint->valuestring != nullptr &&
      candidate.fingerprint == fingerprint->valuestring;

  if (ok && verified_name != nullptr) *verified_name = display_name->valuestring;
  cJSON_Delete(root);

  if (!ok) {
    ESP_LOGE(kTag, "Hub public identity does not match Bonjour advertisement");
    return ESP_ERR_INVALID_RESPONSE;
  }

  if (!body.date_header.empty()) {
    const esp_err_t clock_err =
        update_trusted_clock_from_http_date(body.date_header.c_str());
    if (clock_err == ESP_OK) {
      ESP_LOGI(kTag, "trusted UTC synchronized from verified Hub");
    } else {
      ESP_LOGW(kTag, "verified Hub Date header was not usable");
    }
  } else {
    ESP_LOGW(kTag, "verified Hub response omitted Date header");
  }
  return ESP_OK;
}

}  // namespace

esp_err_t discover_and_verify_hub(FoundationStore *store, VerifiedHub *hub) {
  if (store == nullptr || hub == nullptr) return ESP_ERR_INVALID_ARG;

  esp_err_t err = mdns_init();
  if (err != ESP_OK && err != ESP_ERR_INVALID_STATE) return err;

  mdns_result_t *results = nullptr;
  err = mdns_query_ptr(kService, kProto, kDiscoveryTimeoutMS,
                       kMaxDiscoveryResults, &results);
  if (err != ESP_OK) return err;

  HubBinding remembered;
  err = store->LoadHubBinding(&remembered);
  if (err != ESP_OK) {
    mdns_query_results_free(results);
    return err;
  }

  std::vector<Candidate> candidates;
  for (mdns_result_t *item = results; item != nullptr; item = item->next) {
    Candidate candidate;
    if (!parse_candidate(item, &candidate)) continue;
    if (remembered.complete() && !binding_matches(remembered, candidate)) continue;
    candidates.push_back(std::move(candidate));
  }
  mdns_query_results_free(results);

  if (candidates.size() != 1) {
    ESP_LOGE(kTag, "expected exactly one eligible StageCore Hub, found %u",
             static_cast<unsigned>(candidates.size()));
    return ESP_ERR_INVALID_STATE;
  }

  Candidate candidate = std::move(candidates.front());
  std::vector<unsigned char> certificate;
  err = capture_pinned_certificate(candidate, &certificate);
  if (err != ESP_OK) return err;

  std::string verified_name;
  err = verify_public_identity(candidate, certificate, &verified_name);
  if (err != ESP_OK) return err;

  if (!remembered.complete()) {
    HubBinding binding;
    binding.hub_id = candidate.hub_id;
    binding.fingerprint = candidate.fingerprint;
    binding.tls_sha256 = candidate.tls_sha256;
    err = store->SaveHubBinding(binding);
    if (err != ESP_OK) return err;
    ESP_LOGI(kTag, "remembered verified StageCore Hub %s",
             candidate.hub_id.c_str());
  }

  hub->hub_id = candidate.hub_id;
  hub->display_name = verified_name;
  hub->fingerprint = candidate.fingerprint;
  hub->tls_sha256 = candidate.tls_sha256;
  hub->address = candidate.address;
  hub->port = candidate.port;
  hub->certificate_der = std::move(certificate);

  ESP_LOGI(kTag, "verified StageCore Hub %s at %s:%u",
           hub->display_name.c_str(), hub->address.c_str(), hub->port);
  return ESP_OK;
}

}  // namespace stagecore
