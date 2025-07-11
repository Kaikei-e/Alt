"use client";

import React, { useEffect, useState, useCallback } from 'react';
import { Box, Text, VStack, HStack, Badge, Collapse, IconButton } from '@chakra-ui/react';
import { ChevronDown, ChevronUp, Activity } from 'lucide-react';

interface PerformanceMetrics {
  renderTime: number;
  componentCount: number;
  memoryUsage: number;
  domNodes: number;
  lastUpdate: number;
}

interface PerformanceMonitorProps {
  componentName: string;
  enabled?: boolean;
  children?: React.ReactNode;
}

// Performance monitoring hook
export const usePerformanceProfiler = (componentName: string) => {
  const [metrics, setMetrics] = useState<PerformanceMetrics>({
    renderTime: 0,
    componentCount: 0,
    memoryUsage: 0,
    domNodes: 0,
    lastUpdate: Date.now(),
  });

  const startProfiler = useCallback(() => {
    const startTime = performance.now();
    return () => {
      const endTime = performance.now();
      const renderTime = endTime - startTime;
      
      // Update metrics
      setMetrics(prev => ({
        ...prev,
        renderTime,
        componentCount: prev.componentCount + 1,
        memoryUsage: (performance as any).memory?.usedJSHeapSize || 0,
        domNodes: document.querySelectorAll('*').length,
        lastUpdate: Date.now(),
      }));
    };
  }, []);

  const resetMetrics = useCallback(() => {
    setMetrics({
      renderTime: 0,
      componentCount: 0,
      memoryUsage: 0,
      domNodes: 0,
      lastUpdate: Date.now(),
    });
  }, []);

  return { metrics, startProfiler, resetMetrics };
};

// Performance monitor component
export const PerformanceMonitor: React.FC<PerformanceMonitorProps> = ({
  componentName,
  enabled = process.env.NODE_ENV === 'development',
  children,
}) => {
  const [isExpanded, setIsExpanded] = useState(false);
  const [webVitals, setWebVitals] = useState<{ [key: string]: number }>({});
  const { metrics, startProfiler, resetMetrics } = usePerformanceProfiler(componentName);

  // Track render performance
  useEffect(() => {
    if (!enabled) return;
    
    const endProfiler = startProfiler();
    return endProfiler;
  });

  // Monitor Web Vitals
  useEffect(() => {
    if (!enabled) return;

    const observer = new PerformanceObserver((list) => {
      const entries = list.getEntries();
      entries.forEach((entry) => {
        if (entry.entryType === 'largest-contentful-paint') {
          setWebVitals(prev => ({ ...prev, LCP: entry.startTime }));
        }
        if (entry.entryType === 'first-input') {
          setWebVitals(prev => ({ ...prev, FID: entry.processingStart - entry.startTime }));
        }
        if (entry.entryType === 'layout-shift') {
          setWebVitals(prev => ({ 
            ...prev, 
            CLS: (prev.CLS || 0) + (entry as any).value 
          }));
        }
      });
    });

    observer.observe({ entryTypes: ['largest-contentful-paint', 'first-input', 'layout-shift'] });

    return () => observer.disconnect();
  }, [enabled]);

  // Format bytes
  const formatBytes = (bytes: number) => {
    if (bytes === 0) return '0 Bytes';
    const k = 1024;
    const sizes = ['Bytes', 'KB', 'MB', 'GB'];
    const i = Math.floor(Math.log(bytes) / Math.log(k));
    return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
  };

  // Get performance status
  const getPerformanceStatus = () => {
    const lcp = webVitals.LCP || 0;
    const fid = webVitals.FID || 0;
    const cls = webVitals.CLS || 0;
    
    if (lcp > 4000 || fid > 300 || cls > 0.25) return 'poor';
    if (lcp > 2500 || fid > 100 || cls > 0.1) return 'needs-improvement';
    return 'good';
  };

  const performanceStatus = getPerformanceStatus();

  if (!enabled) {
    return <>{children}</>;
  }

  return (
    <>
      {children}
      
      {/* Performance Monitor Widget */}
      <Box
        position="fixed"
        bottom={4}
        right={4}
        zIndex={9999}
        className="glass"
        borderRadius="var(--radius-lg)"
        border="1px solid var(--surface-border)"
        minW="300px"
        maxW="400px"
        boxShadow="0 8px 32px rgba(0, 0, 0, 0.1)"
      >
        {/* Header */}
        <HStack
          p={3}
          cursor="pointer"
          onClick={() => setIsExpanded(!isExpanded)}
          justify="space-between"
          align="center"
        >
          <HStack gap={2}>
            <Activity size={16} color="var(--accent-primary)" />
            <Text fontSize="sm" fontWeight="semibold" color="var(--text-primary)">
              Performance Monitor
            </Text>
            <Badge
              size="sm"
              colorScheme={
                performanceStatus === 'good' ? 'green' :
                performanceStatus === 'needs-improvement' ? 'yellow' : 'red'
              }
            >
              {performanceStatus}
            </Badge>
          </HStack>
          <IconButton
            aria-label="Toggle performance details"
            size="sm"
            variant="ghost"
            color="var(--text-secondary)"
          >
            {isExpanded ? <ChevronUp size={16} /> : <ChevronDown size={16} />}
          </IconButton>
        </HStack>

        {/* Expandable Details */}
        <Collapse in={isExpanded}>
          <VStack p={4} pt={0} align="stretch" gap={3}>
            {/* Component Info */}
            <Box>
              <Text fontSize="xs" color="var(--text-secondary)" mb={1}>
                Component: {componentName}
              </Text>
              <HStack fontSize="xs" color="var(--text-primary)" justify="space-between">
                <Text>Renders: {metrics.componentCount}</Text>
                <Text>Last: {metrics.renderTime.toFixed(2)}ms</Text>
              </HStack>
            </Box>

            {/* Memory Usage */}
            <Box>
              <HStack fontSize="xs" color="var(--text-primary)" justify="space-between">
                <Text>Memory: {formatBytes(metrics.memoryUsage)}</Text>
                <Text>DOM: {metrics.domNodes} nodes</Text>
              </HStack>
            </Box>

            {/* Web Vitals */}
            <Box>
              <Text fontSize="xs" color="var(--text-secondary)" mb={1}>
                Web Vitals
              </Text>
              <VStack gap={1} fontSize="xs">
                <HStack justify="space-between" width="100%">
                  <Text>LCP:</Text>
                  <Text color={webVitals.LCP > 2500 ? 'red' : 'green'}>
                    {webVitals.LCP ? `${webVitals.LCP.toFixed(0)}ms` : 'N/A'}
                  </Text>
                </HStack>
                <HStack justify="space-between" width="100%">
                  <Text>FID:</Text>
                  <Text color={webVitals.FID > 100 ? 'red' : 'green'}>
                    {webVitals.FID ? `${webVitals.FID.toFixed(2)}ms` : 'N/A'}
                  </Text>
                </HStack>
                <HStack justify="space-between" width="100%">
                  <Text>CLS:</Text>
                  <Text color={webVitals.CLS > 0.1 ? 'red' : 'green'}>
                    {webVitals.CLS ? webVitals.CLS.toFixed(3) : 'N/A'}
                  </Text>
                </HStack>
              </VStack>
            </Box>

            {/* Actions */}
            <HStack justify="space-between" pt={2}>
              <button
                onClick={resetMetrics}
                style={{
                  padding: "4px 8px",
                  borderRadius: "var(--radius-md)",
                  backgroundColor: "var(--surface-border)",
                  color: "var(--text-secondary)",
                  border: "none",
                  fontSize: "10px",
                  fontWeight: "600",
                  cursor: "pointer",
                  transition: "all 0.2s ease",
                }}
              >
                Reset
              </button>
              <Text fontSize="xs" color="var(--text-muted)">
                Dev Mode Only
              </Text>
            </HStack>
          </VStack>
        </Collapse>
      </Box>
    </>
  );
};

export default PerformanceMonitor;