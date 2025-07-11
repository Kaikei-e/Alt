export interface MeasurementResult {
  height: number;
  width: number;
  timestamp: number;
}

export class SizeMeasurementManager {
  private measurementCache = new Map<string, MeasurementResult>();
  private pendingMeasurements = new Set<string>();
  private errorCount = 0;
  private maxErrors = 10;

  constructor(private onError?: (error: Error) => void) {}

  async measureElement(
    element: HTMLElement,
    key: string
  ): Promise<MeasurementResult | null> {
    try {
      // 既にキャッシュに存在する場合
      const cached = this.measurementCache.get(key);
      if (cached && Date.now() - cached.timestamp < 5000) {
        return cached;
      }

      // 測定中の場合はスキップ
      if (this.pendingMeasurements.has(key)) {
        return cached || null;
      }

      this.pendingMeasurements.add(key);

      // 要素が表示状態でない場合は測定を延期
      if (!element.offsetParent) {
        this.pendingMeasurements.delete(key);
        return null;
      }

      const rect = element.getBoundingClientRect();
      const result: MeasurementResult = {
        height: rect.height,
        width: rect.width,
        timestamp: Date.now()
      };

      this.measurementCache.set(key, result);
      this.pendingMeasurements.delete(key);
      
      return result;
    } catch (error) {
      this.errorCount++;
      this.pendingMeasurements.delete(key);
      
      if (this.errorCount > this.maxErrors) {
        this.onError?.(new Error('Too many measurement errors'));
        return null;
      }

      console.warn('Size measurement failed:', error);
      return null;
    }
  }

  clearCache(): void {
    this.measurementCache.clear();
    this.pendingMeasurements.clear();
  }

  getEstimatedSize(contentLength: number): number {
    // コンテンツの長さに基づいた推定サイズ
    const baseHeight = 120;
    const lineHeight = 24;
    const charactersPerLine = 50;
    const estimatedLines = Math.ceil(contentLength / charactersPerLine);
    
    return baseHeight + (estimatedLines * lineHeight);
  }
}