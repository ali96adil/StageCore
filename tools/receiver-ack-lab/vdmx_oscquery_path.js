"use strict";

// Conservative OSCQuery FULL_PATH grammar for this read-only VDMX lab.
// VDMX 6 publishes labels such as "/OSCQUERY/Control Surface/Slider":
// literal ASCII spaces are valid inside an address segment, but need
// percent-encoding in the HTTP request line. Reject percent signs so
// no caller can supply encoded traversal or ambiguously double-encode.
const SEGMENT = /^[A-Za-z0-9_-](?:[A-Za-z0-9_. -]*[A-Za-z0-9_-])?$/;
function validPublishedPath(path, allowRoot = false) {
  if (path === "/") return allowRoot === true;
  if (typeof path !== "string" || !path.startsWith("/") ||
      path.length < 2 || Buffer.byteLength(path, "utf8") > 121) {
    return false;
  }
  const parts = path.slice(1).split("/");
  return parts.every((part) => part.length <= 80 && SEGMENT.test(part));
}
function validChildName(child) {
  return typeof child === "string" && child.length <= 80 &&
    SEGMENT.test(child);
}
function encodePublishedGETPath(path) {
  if (!validPublishedPath(path)) throw new Error("invalid published OSCQuery path");
  return path.split("/").map((segment) => encodeURIComponent(segment)).join("/");
}
module.exports = { validPublishedPath, validChildName, encodePublishedGETPath };
