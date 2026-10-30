// Trusting the server's own CA, when it serves -https with it: a device
// trusts it once, on the trust page, before the https address opens there
// without a warning and the page installs as an app.

// shortFingerprint is the start of the CA's fingerprint, enough to check at
// a glance; the trust page shows all of it.
export function shortFingerprint(fingerprint: string): string {
  const pairs = fingerprint.split(":");
  return pairs.length > 8 ? pairs.slice(0, 8).join(":") + "…" : fingerprint;
}

// httpsAddress is where a page open over plain HTTP is over https: the same
// host and port, as one port serves both. Without a port it came to http's
// own, 80, which https is on too, not https's own, 443.
export function httpsAddress(page: Pick<Location, "protocol" | "host" | "port">): string {
  const port = page.port === "" && page.protocol === "http:" ? ":80" : "";
  return `https://${page.host}${port}`;
}
