export class WaitTracker {
  private timestamps: number[] = [];

  constructor(private thresholdMs = 5000) {}

  record(waitMs: number) {
    const now = Date.now();
    this.timestamps = this.timestamps
      .filter((ts) => now - ts < this.thresholdMs)
      .concat(now);

    if (this.timestamps.length > 5) {
      console.warn(
        `[WaitTracker] Excessive waits detected within ${this.thresholdMs}ms window`,
      );
    }
  }
}

