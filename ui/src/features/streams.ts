// A feature owns the streams whose names start with its prefix and links
// each to its own page. The task page's "Made from" goes
// through featureLink, so it names no feature itself.

export interface FeatureStreams {
  prefix: string; // the whole prefix, with its "-"
  to: (stream: string) => string; // the page that shows the stream
}

// featureLink is the link the first owning feature gives the stream, or
// undefined when no feature owns it. A stream is owned only if something
// follows the prefix, so the prefix alone is nobody's.
export function featureLink(stream: string, features: { streams?: FeatureStreams }[]): string | undefined {
  const owner = features.find(({ streams }) => streams && stream.length > streams.prefix.length && stream.startsWith(streams.prefix));
  return owner?.streams?.to(stream);
}
