// AGENTS is how the page names the agents' stream of the log, which holds
// their roster as well as what they learned.
export const AGENTS = "the agents and what they learned";

// streamName names a stream of the log for the page: the agents' by what
// it holds, any other, such as a task's, by its id.
export const streamName = (stream: string) => (stream === "agents" ? AGENTS : stream);
