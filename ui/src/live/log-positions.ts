// The server's log only grows, so a position below one it has already said
// means it restarted on another log: a restored or different database, or a
// test server. What the page read from the old log may hold events the new
// one doesn't have, so it is read again. LogPositions tells the resource
// cache and the event pages when that happens.

// Sent is where the log was when a read went out, to check its reply against.
export interface Sent {
  generation: number;
  highest: number;
}

export class LogPositions {
  private highest = 0; // the highest position the server has said
  private generation = 0; // counts resets: a read sent before one is thrown away

  // reset is called with the new log's position when it is found shorter.
  constructor(private reset: (position: number) => void) {}

  sent(): Sent {
    return { generation: this.generation, highest: this.highest };
  }

  // moved takes the poll's position. It returns false when the log is
  // shorter: it has reset instead.
  moved(position: number): boolean {
    return this.replied(this.sent(), position);
  }

  // replied takes the position a reply was read at, if it said, and returns
  // whether the reply is of the log now held. One below what the server
  // said before the read went out is of a shorter log: it resets, and the
  // reply is thrown away with every other one sent before.
  replied(sent: Sent, position?: number): boolean {
    if (this.stale(sent)) return false;
    if (position === undefined) return true;
    if (position < sent.highest) {
      this.generation++;
      this.highest = position; // so the reset is done once, and the new log is followed from here
      this.reset(position);
      return false;
    }
    this.highest = Math.max(this.highest, position);
    return true;
  }

  // stale says whether a read was sent before a reset.
  stale(sent: Sent): boolean {
    return sent.generation !== this.generation;
  }
}
