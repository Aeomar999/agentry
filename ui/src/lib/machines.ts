// How the page names machines: the one Agentry runs on is "local" to the
// server, and "this machine" to you.

export const LOCAL = "local";

export function machineName(name: string): string {
  return !name || name === LOCAL ? "this machine" : name;
}

// Where an agent runs, in its order: the first that is online wins.
export function runsOn(workers: string[]): string {
  return workers.length ? workers.map(machineName).join(", then ") : machineName(LOCAL);
}

// Bytes as people read them: 512 MB, 64 GB, 1.8 TB.
export function bytes(n: number): string {
  const units = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1000 && i < units.length - 1) {
    n /= 1000;
    i++;
  }
  return `${n >= 100 || i === 0 ? Math.round(n) : n.toFixed(1).replace(/\.0$/, "")} ${units[i]}`;
}

// onMachine is an agent, and the machine its coordinator sent it to, if it chose one.
export function onMachine(agent: string, machine: string): string {
  return machine ? `${agent} on ${machine === "local" ? "this machine" : machine}` : agent;
}
