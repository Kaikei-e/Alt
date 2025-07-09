import { test, expect } from "@playwright/test";
import { mockApiEndpoints } from "../../helpers/mockApi";

test.describe("Feeds Stats Page - Comprehensive Tests", () => {
  test.beforeEach(async ({ page }) => {
    // Mock API endpoints
    await mockApiEndpoints(page, {});

    // Enhanced EventSource mocking with correct data structure
    await page.addInitScript(() => {
      // Override EventSource with a more robust mock
      (window as any).EventSource = class MockEventSource {
        private url: string;
        private callbacks: { [key: string]: Function[] } = {};
        private readyState: number = 0;
        onopen: Function | null = null;
        onmessage: Function | null = null;
        onerror: Function | null = null;

        constructor(url: string) {
          this.url = url;
          this.callbacks = {};
          console.log("EventSource mock created for:", url);

          // Immediately set to connecting state
          this.readyState = 0;

          // Simulate connection opening
          setTimeout(() => {
            this.readyState = 1;
            console.log("EventSource mock opened");
            if (this.onopen) {
              this.onopen(new Event("open"));
            }
            this.dispatchEvent(new Event("open"));

            // Send initial data after connection opens
            setTimeout(() => {
              this.sendMockData();
            }, 200);
          }, 100);
        }

        private sendMockData() {
          // Send stats data that matches the actual useSSEFeedsStats expectations
          const statsData = {
            feed_amount: { amount: 25 },
            unsummarized_feed: { amount: 18 },
            total_articles: { amount: 1337 },
          };

          const event = new MessageEvent("message", {
            data: JSON.stringify(statsData),
            origin: this.url,
          });

          console.log("Sending SSE data:", statsData);

          if (this.onmessage) {
            this.onmessage(event);
          }

          // Trigger any registered message listeners
          if (this.callbacks["message"]) {
            this.callbacks["message"].forEach((callback) => callback(event));
          }
        }

        sendUpdateData() {
          // Send updated data for the update test
          const updatedData = {
            feed_amount: { amount: 30 },
            unsummarized_feed: { amount: 22 },
            total_articles: { amount: 1500 },
          };

          const event = new MessageEvent("message", {
            data: JSON.stringify(updatedData),
            origin: this.url,
          });

          console.log("Sending SSE update data:", updatedData);

          if (this.onmessage) {
            this.onmessage(event);
          }

          if (this.callbacks["message"]) {
            this.callbacks["message"].forEach((callback) => callback(event));
          }
        }

        addEventListener(type: string, listener: Function) {
          if (!this.callbacks[type]) {
            this.callbacks[type] = [];
          }
          this.callbacks[type].push(listener);
        }

        removeEventListener(type: string, listener: Function) {
          if (this.callbacks[type]) {
            this.callbacks[type] = this.callbacks[type].filter(
              (l) => l !== listener,
            );
          }
        }

        dispatchEvent(event: Event) {
          const type = event.type;
          if (this.callbacks[type]) {
            this.callbacks[type].forEach((callback) => callback(event));
          }
          return true;
        }

        close() {
          this.readyState = 2;
          console.log("EventSource mock closed");
        }

        get CONNECTING() {
          return 0;
        }
        get OPEN() {
          return 1;
        }
        get CLOSED() {
          return 2;
        }
      };

      // Store reference for tests to trigger updates
      (window as any).triggerSSEUpdate = function () {
        const connections = (window as any).eventSourceConnections || [];
        connections.forEach((connection: any) => {
          if (connection.sendUpdateData) {
            connection.sendUpdateData();
          }
        });
      };
    });

    // Track EventSource connections
    await page.addInitScript(() => {
      const originalEventSource = (window as any).EventSource;
      (window as any).EventSource = class extends originalEventSource {
        constructor(url: string) {
          super(url);
          if (!(window as any).eventSourceConnections) {
            (window as any).eventSourceConnections = [];
          }
          (window as any).eventSourceConnections.push(this);
        }
      };
    });
  });

  test.describe("SSE Data Loading", () => {
    test("should display correct feed amounts from SSE", async ({ page }) => {
      await page.goto("/mobile/feeds/stats", {
        waitUntil: "domcontentloaded",
        timeout: 30000,
      });

      // Wait for page to load
      await page.waitForSelector("body", { timeout: 15000 });

      // Wait for EventSource to connect and send data
      await page.waitForTimeout(1500);

      // Check that the values from our mock are displayed
      // Numbers are formatted with toLocaleString(), so 1337 becomes "1,337"
      await expect(page.getByText("25")).toBeVisible({ timeout: 15000 });
      await expect(page.getByText("18")).toBeVisible({ timeout: 10000 });
      await expect(page.getByText("1,337")).toBeVisible({ timeout: 10000 });
    });

    test("should handle initial zero values", async ({ page }) => {
      // Mock EventSource to send zero values initially
      await page.addInitScript(() => {
        const originalEventSource = (window as any).EventSource;
        (window as any).EventSource = class extends originalEventSource {
          constructor(url: string) {
            super(url);
            setTimeout(() => {
              (this as any).readyState = 1;
              if ((this as any).onopen) (this as any).onopen(new Event("open"));

              setTimeout(() => {
                const event = new MessageEvent("message", {
                  data: JSON.stringify({
                    feed_amount: { amount: 0 },
                    unsummarized_feed: { amount: 0 },
                    total_articles: { amount: 0 },
                  }),
                  origin: url,
                });

                if ((this as any).onmessage) (this as any).onmessage(event);
              }, 100);
            }, 100);
          }
        };
      });

      await page.goto("/mobile/feeds/stats", {
        waitUntil: "domcontentloaded",
        timeout: 30000,
      });

      await page.waitForSelector("body", { timeout: 15000 });
      await page.waitForTimeout(1000);

      // Check for zero values - wait for the StatCard components to render
      await page.waitForSelector('[data-testid="stat-card"], .glass', { timeout: 10000 });
      
      // Check for zero values in the formatted number displays
      await expect(page.getByText("0")).toBeVisible({ timeout: 10000 });
    });

    test("should update values when SSE sends new data", async ({ page }) => {
      await page.goto("/mobile/feeds/stats", {
        waitUntil: "domcontentloaded",
        timeout: 30000,
      });

      // Wait for page to load and initial data
      await page.waitForSelector("body", { timeout: 15000 });
      await page.waitForTimeout(1500);

      // Verify initial values are displayed (formatted with commas)
      await expect(page.getByText("25")).toBeVisible({ timeout: 15000 });

      // Trigger SSE update
      await page.evaluate(() => {
        (window as any).triggerSSEUpdate?.();
      });

      // Wait for update to process
      await page.waitForTimeout(1000);

      // Check for updated values (30 and 1500 → "1,500")
      await expect(page.getByText("30")).toBeVisible({ timeout: 15000 });
    });

    test("should handle different data scenarios", async ({ page }) => {
      // Override the EventSource mock completely for this test
      await page.addInitScript(() => {
        // Clear any existing EventSource
        delete (window as any).EventSource;

        // Define fresh EventSource mock with different data
        (window as any).EventSource = class MockEventSource {
          private url: string;
          private readyState: number = 0;
          onopen: Function | null = null;
          onmessage: Function | null = null;
          onerror: Function | null = null;

          constructor(url: string) {
            this.url = url;
            console.log(
              "Different data scenario EventSource created for:",
              url,
            );

            // Simulate connection opening
            setTimeout(() => {
              this.readyState = 1;
              console.log("Different data scenario EventSource opened");
              if (this.onopen) this.onopen(new Event("open"));

              // Send the specific data for this test
              setTimeout(() => {
                const data = {
                  feed_amount: { amount: 15 },
                  unsummarized_feed: { amount: 8 },
                  total_articles: { amount: 456 },
                };

                console.log("Sending different data scenario data:", data);

                const event = new MessageEvent("message", {
                  data: JSON.stringify(data),
                  origin: this.url,
                });

                if (this.onmessage) this.onmessage(event);
              }, 200);
            }, 100);
          }

          close() {
            this.readyState = 2;
            console.log("Different data scenario EventSource closed");
          }

          get CONNECTING() {
            return 0;
          }
          get OPEN() {
            return 1;
          }
          get CLOSED() {
            return 2;
          }
        };
      });

      await page.goto("/mobile/feeds/stats", {
        waitUntil: "domcontentloaded",
        timeout: 30000,
      });

      await page.waitForSelector("body", { timeout: 15000 });

      // Wait longer for the specific data to be sent
      await page.waitForTimeout(2000);

      // Debug: Check current page content
      const bodyText = await page.textContent("body");
      console.log("Page content for different data scenario:", bodyText);

      // Check the different values (all should display without commas since < 1000)
      await expect(page.getByText("15")).toBeVisible({ timeout: 10000 });
      await expect(page.getByText("8")).toBeVisible({ timeout: 10000 });
      await expect(page.getByText("456")).toBeVisible({ timeout: 10000 });
    });

    test("should handle SSE connection errors gracefully", async ({ page }) => {
      // Mock EventSource to simulate error
      await page.addInitScript(() => {
        (window as any).EventSource = class {
          constructor(url: string) {
            setTimeout(() => {
              (this as any).readyState = 2;
              if ((this as any).onerror) {
                (this as any).onerror(new Event("error"));
              }
            }, 100);
          }

          close() {}
          addEventListener() {}
          removeEventListener() {}
        };
      });

      await page.goto("/mobile/feeds/stats", {
        waitUntil: "domcontentloaded",
        timeout: 30000,
      });

      await page.waitForSelector("body", { timeout: 15000 });
      await page.waitForTimeout(1000);

      // Should still render the page without crashing
      await expect(page.locator("body")).toBeVisible();
    });
  });
});
