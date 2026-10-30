// An extension owns the streams whose names start with its prefix and links
// each to its own page. The task page's "Made from" goes
// through extensionLink, so it names no extension itself.

export interface ExtensionStreams {
  prefix: string; // the whole prefix, with its "-"
  to: (stream: string) => string; // the page that shows the stream
}

// extensionLink is the link the first owning extension gives the stream, or
// undefined when no extension owns it. A stream is owned only if something
// follows the prefix, so the prefix alone is nobody's.
export function extensionLink(stream: string, extensions: { streams?: ExtensionStreams }[]): string | undefined {
  const owner = extensions.find(({ streams }) => streams && stream.length > streams.prefix.length && stream.startsWith(streams.prefix));
  return owner?.streams?.to(stream);
}
