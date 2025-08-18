/**
 * Security Monitoring and Anomaly Detection System - 2025 Edition
 * Real-time security event correlation and threat detection for logging system
 */

import { StructuredLogger, createComponentLogger } from "../utils/logger.ts";

/**
 * Security event types for monitoring
 */
export type SecurityEvent = 
  | 'unauthorized_access'
  | 'suspicious_pattern'
  | 'rate_limit_exceeded'
  | 'data_breach_attempt'
  | 'log_tampering'
  | 'privilege_escalation'
  | 'anomalous_behavior'
  | 'compliance_violation';

/**
 * Threat levels for security events
 */
export type ThreatLevel = 'low' | 'medium' | 'high' | 'critical';

/**
 * Security metrics for monitoring
 */
export interface SecurityMetrics {
  timestamp: string;
  event_type: SecurityEvent;
  threat_level: ThreatLevel;
  source_ip?: string;
  user_id?: string;
  component: string;
  details: Record<string, unknown>;
  correlation_id: string;
}

/**
 * Anomaly detection patterns
 */
interface AnomalyPattern {
  name: string;
  description: string;
  threshold: number;
  timeWindow: number; // in seconds
  severity: ThreatLevel;
}

/**
 * Security monitoring configuration
 */
interface SecurityConfig {
  enabled: boolean;
  realTimeAlerts: boolean;
  anomalyDetection: boolean;
  retentionDays: number;
  alertWebhook?: string;
  complianceMode: 'gdpr' | 'ccpa' | 'hipaa' | 'sox' | 'pci';
}

/**
 * Advanced Security Monitoring System
 */
export class SecurityMonitor {
  private static instance: SecurityMonitor;
  private logger: StructuredLogger;
  private config: SecurityConfig;
  private eventBuffer: SecurityMetrics[] = [];
  private anomalyPatterns: Map<string, AnomalyPattern> = new Map();
  private alertHistory: Map<string, number> = new Map();
  
  private constructor() {
    this.logger = createComponentLogger('security-monitor');
    this.config = this.loadConfiguration();
    this.initializeAnomalyPatterns();
    this.startMonitoring();
  }

  static getInstance(): SecurityMonitor {
    if (!SecurityMonitor.instance) {
      SecurityMonitor.instance = new SecurityMonitor();
    }
    return SecurityMonitor.instance;
  }

  /**
   * Load security monitoring configuration
   */
  private loadConfiguration(): SecurityConfig {
    return {
      enabled: Deno.env.get('SECURITY_MONITORING_ENABLED') === 'true',
      realTimeAlerts: Deno.env.get('REAL_TIME_ALERTS') === 'true',
      anomalyDetection: Deno.env.get('ANOMALY_DETECTION_ENABLED') === 'true',
      retentionDays: parseInt(Deno.env.get('SECURITY_RETENTION_DAYS') || '90'),
      alertWebhook: Deno.env.get('SECURITY_ALERT_WEBHOOK'),
      complianceMode: (Deno.env.get('COMPLIANCE_MODE') as any) || 'gdpr'
    };
  }

  /**
   * Initialize anomaly detection patterns
   */
  private initializeAnomalyPatterns(): void {
    const patterns: AnomalyPattern[] = [
      {
        name: 'rapid_failed_auth',
        description: 'Multiple authentication failures in short time',
        threshold: 5,
        timeWindow: 300, // 5 minutes
        severity: 'high'
      },
      {
        name: 'unusual_access_pattern',
        description: 'Access from unusual IP or location',
        threshold: 3,
        timeWindow: 3600, // 1 hour
        severity: 'medium'
      },
      {
        name: 'privilege_escalation_attempt',
        description: 'Attempts to access restricted resources',
        threshold: 2,
        timeWindow: 600, // 10 minutes
        severity: 'critical'
      }
    ];

    patterns.forEach(pattern => {
      this.anomalyPatterns.set(pattern.name, pattern);
    });
  }

  /**
   * Record security event for monitoring
   */
  recordSecurityEvent(
    eventType: SecurityEvent,
    threatLevel: ThreatLevel,
    details: Record<string, unknown>,
    component = 'auth-token-manager'
  ): void {
    if (!this.config.enabled) return;

    const event: SecurityMetrics = {
      timestamp: new Date().toISOString(),
      event_type: eventType,
      threat_level: threatLevel,
      source_ip: details.source_ip as string,
      user_id: details.user_id as string,
      component,
      details,
      correlation_id: this.generateCorrelationId()
    };

    this.eventBuffer.push(event);
    this.processSecurityEvent(event);

    // Log the security event
    this.logger.logSecurity(`Security event: ${eventType}`, {
      threat_level: threatLevel,
      correlation_id: event.correlation_id,
      ...details
    });
  }

  /**
   * Process security event and check for anomalies
   */
  private processSecurityEvent(event: SecurityMetrics): void {
    if (this.config.anomalyDetection) {
      this.detectAnomalies(event);
    }

    if (this.config.realTimeAlerts && (event.threat_level === 'high' || event.threat_level === 'critical')) {
      this.sendRealTimeAlert(event);
    }
  }

  /**
   * Detect anomalies based on patterns
   */
  private detectAnomalies(event: SecurityMetrics): void {
    const now = Date.now();
    
    for (const [patternName, pattern] of this.anomalyPatterns.entries()) {
      const recentEvents = this.eventBuffer.filter(e => {
        const eventTime = new Date(e.timestamp).getTime();
        const isRecent = (now - eventTime) <= (pattern.timeWindow * 1000);
        return isRecent && this.eventMatchesPattern(e, patternName, pattern);
      });

      if (recentEvents.length >= pattern.threshold) {
        this.triggerAnomalyAlert(patternName, pattern, recentEvents);
      }
    }
  }

  /**
   * Check if event matches anomaly pattern
   */
  private eventMatchesPattern(
    event: SecurityMetrics,
    patternName: string,
    _pattern: AnomalyPattern
  ): boolean {
    switch (patternName) {
      case 'rapid_failed_auth':
        return event.event_type === 'unauthorized_access' && 
               event.details.auth_result === 'failed';
      
      case 'unusual_access_pattern':
        return event.source_ip !== undefined &&
               this.isUnusualIP(event.source_ip);
      
      case 'privilege_escalation_attempt':
        return event.event_type === 'privilege_escalation';
      
      default:
        return false;
    }
  }

  /**
   * Check if IP address is unusual
   */
  private isUnusualIP(ip: string): boolean {
    const knownGoodIPs = ['192.168.', '10.0.', '172.16.'];
    return !knownGoodIPs.some(range => ip.startsWith(range));
  }

  /**
   * Trigger anomaly alert
   */
  private triggerAnomalyAlert(
    patternName: string,
    pattern: AnomalyPattern,
    events: SecurityMetrics[]
  ): void {
    const alertKey = `${patternName}_${Date.now()}`;
    
    // Prevent alert spam
    const lastAlert = this.alertHistory.get(patternName) || 0;
    const cooldownPeriod = 300000; // 5 minutes
    
    if (Date.now() - lastAlert < cooldownPeriod) {
      return;
    }

    this.alertHistory.set(patternName, Date.now());

    const alert = {
      alert_id: alertKey,
      pattern_name: patternName,
      description: pattern.description,
      severity: pattern.severity,
      event_count: events.length,
      time_window: pattern.timeWindow,
      first_event: events[0].timestamp,
      last_event: events[events.length - 1].timestamp,
    };

    this.logger.critical(`Security anomaly detected: ${patternName}`, alert);
  }

  /**
   * Send real-time alert
   */
  private async sendRealTimeAlert(event: SecurityMetrics): Promise<void> {
    if (!this.config.alertWebhook) {
      return;
    }

    try {
      const alertPayload = {
        timestamp: event.timestamp,
        event_type: event.event_type,
        threat_level: event.threat_level,
        component: event.component,
        correlation_id: event.correlation_id,
        summary: `Security alert: ${event.event_type} (${event.threat_level})`,
        details: event.details
      };

      await fetch(this.config.alertWebhook, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          'User-Agent': 'auth-token-manager-security-monitor/1.0'
        },
        body: JSON.stringify(alertPayload)
      });

      this.logger.info('Real-time alert sent successfully', {
        correlation_id: event.correlation_id
      });
    } catch (error) {
      this.logger.error('Failed to send real-time alert', {
        error: error instanceof Error ? error.message : String(error),
        correlation_id: event.correlation_id
      });
    }
  }

  /**
   * Generate correlation ID for event tracking
   */
  private generateCorrelationId(): string {
    const array = new Uint8Array(16);
    crypto.getRandomValues(array);
    return Array.from(array, byte => byte.toString(16).padStart(2, '0')).join('');
  }

  /**
   * Get security metrics summary
   */
  getSecurityMetrics(timeRange = 3600): {
    total_events: number;
    events_by_type: Record<string, number>;
    events_by_severity: Record<string, number>;
    anomalies_detected: number;
  } {
    const now = Date.now();
    const cutoff = now - (timeRange * 1000);
    
    const recentEvents = this.eventBuffer.filter(e => 
      new Date(e.timestamp).getTime() >= cutoff
    );

    const eventsByType = {} as Record<string, number>;
    const eventsBySeverity = {} as Record<string, number>;
    let anomalies = 0;

    recentEvents.forEach(event => {
      eventsByType[event.event_type] = (eventsByType[event.event_type] || 0) + 1;
      eventsBySeverity[event.threat_level] = (eventsBySeverity[event.threat_level] || 0) + 1;
      
      if (event.event_type === 'anomalous_behavior') anomalies++;
    });

    return {
      total_events: recentEvents.length,
      events_by_type: eventsByType,
      events_by_severity: eventsBySeverity,
      anomalies_detected: anomalies
    };
  }

  /**
   * Start monitoring process
   */
  private startMonitoring(): void {
    if (!this.config.enabled) return;

    this.logger.info('Security monitoring started', {
      enabled: this.config.enabled,
      real_time_alerts: this.config.realTimeAlerts,
      anomaly_detection: this.config.anomalyDetection,
      compliance_mode: this.config.complianceMode
    });
  }
}

// Export singleton instance
export const securityMonitor = SecurityMonitor.getInstance();