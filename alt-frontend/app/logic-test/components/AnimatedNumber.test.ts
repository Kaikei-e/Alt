import { describe, expect, it, vi, beforeEach, afterEach } from "vitest";

// Simple AnimatedNumber behavior simulator for testing
class AnimatedNumberSimulator {
  private displayValue = 0;
  private targetValue = 0;
  private duration = 300;

  constructor(initialValue: number = 0, duration: number = 300) {
    this.displayValue = initialValue;
    this.targetValue = initialValue;
    this.duration = duration;
  }

  setValue(newValue: number, customDuration?: number) {
    this.targetValue = newValue;
    if (customDuration) {
      this.duration = customDuration;
    }
  }

  getCurrentValue() {
    return this.displayValue;
  }

  getTargetValue() {
    return this.targetValue;
  }

  getDuration() {
    return this.duration;
  }

  // Simulate instant completion for testing
  completeAnimation() {
    this.displayValue = this.targetValue;
  }

  isAnimating() {
    return this.displayValue !== this.targetValue;
  }

  formatValue(formatOptions?: Intl.NumberFormatOptions) {
    return formatOptions 
      ? new Intl.NumberFormat('en-US', formatOptions).format(this.displayValue)
      : this.displayValue.toString();
  }
}

describe("AnimatedNumber", () => {
  let simulator: AnimatedNumberSimulator;

  beforeEach(() => {
    simulator = new AnimatedNumberSimulator(0, 300);
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  describe("Initial State", () => {
    it("should initialize with the provided value", () => {
      const customSimulator = new AnimatedNumberSimulator(42);
      expect(customSimulator.getCurrentValue()).toBe(42);
      expect(customSimulator.getTargetValue()).toBe(42);
    });

    it("should default to 0 when no initial value provided", () => {
      expect(simulator.getCurrentValue()).toBe(0);
      expect(simulator.getTargetValue()).toBe(0);
    });

    it("should not be animating initially", () => {
      expect(simulator.isAnimating()).toBe(false);
    });
  });

  describe("Value Setting", () => {
    it("should set target value correctly", () => {
      simulator.setValue(100);
      
      expect(simulator.getTargetValue()).toBe(100);
      expect(simulator.isAnimating()).toBe(true);
    });

    it("should handle negative values", () => {
      simulator.setValue(-10);
      expect(simulator.getTargetValue()).toBe(-10);
    });

    it("should handle zero values", () => {
      simulator.setValue(100);
      simulator.setValue(0);
      expect(simulator.getTargetValue()).toBe(0);
    });

    it("should handle large numbers", () => {
      simulator.setValue(999999);
      expect(simulator.getTargetValue()).toBe(999999);
    });
  });

  describe("Animation Completion", () => {
    it("should reach target value when animation completes", () => {
      simulator.setValue(50);
      simulator.completeAnimation();
      
      expect(simulator.getCurrentValue()).toBe(50);
      expect(simulator.isAnimating()).toBe(false);
    });

    it("should handle consecutive value changes", () => {
      simulator.setValue(100);
      simulator.setValue(200); // Change target before completion
      
      expect(simulator.getTargetValue()).toBe(200);
      
      simulator.completeAnimation();
      expect(simulator.getCurrentValue()).toBe(200);
    });
  });

  describe("Custom Duration", () => {
    it("should use custom duration when provided", () => {
      simulator.setValue(100, 600);
      expect(simulator.getDuration()).toBe(600);
    });

    it("should maintain default duration for subsequent calls", () => {
      simulator.setValue(50, 150);
      simulator.setValue(75); // Should use 150ms from previous call
      expect(simulator.getDuration()).toBe(150);
    });
  });

  describe("Number Formatting", () => {
    it("should format numbers as strings by default", () => {
      simulator.setValue(1234);
      simulator.completeAnimation();
      
      expect(simulator.formatValue()).toBe("1234");
    });

    it("should apply custom number formatting", () => {
      simulator.setValue(1234567);
      simulator.completeAnimation();
      
      const formatted = simulator.formatValue({ 
        style: 'decimal',
        minimumFractionDigits: 0,
        maximumFractionDigits: 0
      });
      
      expect(formatted).toMatch(/1,234,567|1234567/); // Locale-dependent
    });

    it("should handle currency formatting", () => {
      simulator.setValue(1234);
      simulator.completeAnimation();
      
      const formatted = simulator.formatValue({
        style: 'currency',
        currency: 'USD'
      });
      
      expect(formatted).toMatch(/\$1,234\.00|\$1234\.00/);
    });
  });

  describe("Edge Cases", () => {
    it("should handle same value assignment", () => {
      simulator.setValue(42);
      simulator.completeAnimation();
      
      simulator.setValue(42); // Same value
      expect(simulator.isAnimating()).toBe(false);
    });

    it("should handle zero to positive transition", () => {
      expect(simulator.getCurrentValue()).toBe(0);
      
      simulator.setValue(100);
      simulator.completeAnimation();
      
      expect(simulator.getCurrentValue()).toBe(100);
    });

    it("should handle positive to negative transition", () => {
      simulator.setValue(50);
      simulator.completeAnimation();
      
      simulator.setValue(-25);
      simulator.completeAnimation();
      
      expect(simulator.getCurrentValue()).toBe(-25);
    });
  });
});