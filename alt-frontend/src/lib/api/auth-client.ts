import type { User, LoginFlow, RegistrationFlow, UserPreferences } from '@/types/auth';

export class AuthAPIClient {
  private baseURL: string;
  private debugMode: boolean;
  private requestId: number;
  private contentTypeCache: Map<string, string>;
  private cacheExpiry: Map<string, number>;

  constructor() {
    // Use relative API proxy endpoints for secure HTTPS communication
    // This avoids mixed content issues and keeps internal URLs secure
    this.baseURL = '/api/auth';
    this.debugMode = process.env.NODE_ENV === 'development';
    this.requestId = 0;
    this.contentTypeCache = new Map();
    this.cacheExpiry = new Map();
  }

  // 接続テスト機能追加 (X1.md 1.3.2 実装)
  async testConnection(): Promise<boolean> {
    try {
      const response = await fetch(`${this.baseURL}`, {
        method: 'GET',
        signal: AbortSignal.timeout(5000)
      });
      return response.ok;
    } catch {
      return false;
    }
  }

  async initiateLogin(): Promise<LoginFlow> {
    const response = await this.makeRequest('POST', '/login');

    // 防御的プログラミング: レスポンスデータの検証
    if (!response || !response.data || typeof response.data !== 'object') {
      throw new Error('Invalid login flow response format');
    }

    const loginFlow = response.data as LoginFlow;
    if (!loginFlow.id) {
      throw new Error('Login flow response missing required ID');
    }

    return loginFlow;
  }

  async completeLogin(flowId: string, email: string, password: string): Promise<User> {
    console.log('🚀 Starting Kratos-compliant login process...');
    
    // 🎯 Kratosスキーマ完全準拠ログインペイロード生成
    const payload = this.createKratosCompliantLoginPayload(email, password);

    console.log('[AUTH-CLIENT] Sending Kratos-compliant login payload:', {
      flowId: flowId,
      hasIdentifier: !!payload.identifier,
      hasPassword: !!payload.password,
      method: payload.method
    });

    const response = await this.makeRequest('POST', `/login/${flowId}`, payload);
    console.log('✅ Login request completed successfully');
    return response.data as User;
  }

  async initiateRegistration(): Promise<RegistrationFlow> {
    const response = await this.makeRequest('POST', '/register');

    // 防御的プログラミング: レスポンスデータの検証
    if (!response || !response.data || typeof response.data !== 'object') {
      throw new Error('Invalid registration flow response format');
    }

    const registrationFlow = response.data as RegistrationFlow;
    if (!registrationFlow.id) {
      throw new Error('Registration flow response missing required ID');
    }

    return registrationFlow;
  }

  async completeRegistration(flowId: string, email: string, password: string, name?: string): Promise<User> {
    const currentRequestId = ++this.requestId;
    const startTime = performance.now();
    
    // 🔍 ULTRA-DIAGNOSTIC: 完全なリクエスト情報をキャプチャ
    const diagnosticInfo = {
      requestId: `REG-${currentRequestId}`,
      timestamp: new Date().toISOString(),
      userAgent: navigator.userAgent,
      sessionStorage: typeof window !== 'undefined' ? Object.keys(sessionStorage).length : 0,
      localStorage: typeof window !== 'undefined' ? Object.keys(localStorage).length : 0,
      cookieCount: typeof document !== 'undefined' ? document.cookie.split(';').length : 0,
      url: typeof window !== 'undefined' ? window.location.href : 'unknown',
      flowId: {
        provided: !!flowId,
        length: flowId?.length || 0,
        format: flowId ? (flowId.startsWith('flow') ? 'kratos-format' : 'unknown-format') : 'missing'
      },
      email: {
        provided: !!email,
        length: email?.length || 0,
        hasAtSymbol: email?.includes('@') || false,
        domain: email?.split('@')[1] || null
      },
      password: {
        provided: !!password,
        length: password?.length || 0,
        meetsCriteria: password ? password.length >= 8 : false
      },
      name: {
        provided: !!name,
        length: name?.length || 0
      }
    };
    
    console.groupCollapsed(`🔍 [AUTH-CLIENT-DIAGNOSTIC] Registration Request ${diagnosticInfo.requestId}`);
    console.log('📋 Request Diagnostic Info:', diagnosticInfo);
    console.log('🕐 Start Time:', new Date(diagnosticInfo.timestamp).toLocaleTimeString());
    
    // 送信前の詳細検証とログ出力
    console.log('[AUTH-CLIENT] Registration data validation:', {
      flowId: flowId ? 'present' : 'missing',
      email: email ? 'present' : 'missing',
      password: password ? 'present' : 'missing',
      name: name || 'not provided',
      timestamp: diagnosticInfo.timestamp,
      requestId: diagnosticInfo.requestId
    });

    // 基本バリデーション
    if (!flowId || flowId.trim() === '') {
      throw new Error('VALIDATION_FAILED: Flow ID is required');
    }

    if (!email || email.trim() === '' || !email.includes('@')) {
      throw new Error('VALIDATION_FAILED: Valid email address is required');
    }

    if (!password || password.length < 8) {
      throw new Error('VALIDATION_FAILED: Password must be at least 8 characters');
    }

    // 🎯 Kratosスキーマ完全準拠ペイロード生成
    const payload = this.createKratosCompliantRegistrationPayload(email, password, name);

    // 送信前の最終検証ログ
    console.log('[AUTH-CLIENT] Sending Kratos-compliant registration payload:', {
      flowId: flowId,
      payloadStructure: {
        hasTraits: !!payload.traits,
        hasEmail: !!(payload.traits as any)?.email,
        hasName: !!(payload.traits as any)?.name,
        hasPassword: !!payload.password,
        method: payload.method
      },
      payloadSize: JSON.stringify(payload).length
    });

    try {
      const response = await this.makeRequest('POST', `/register/${flowId}`, payload);
      const endTime = performance.now();
      const duration = endTime - startTime;
      
      // 🎉 SUCCESS: 完全な成功レスポンス診断
      const successDiagnostic = {
        requestId: diagnosticInfo.requestId,
        status: 'SUCCESS',
        duration: `${duration.toFixed(2)}ms`,
        responseSize: JSON.stringify(response).length,
        hasData: !!response.data,
        userData: response.data ? {
          hasId: !!(response.data as User).id,
          hasEmail: !!(response.data as User).email,
          hasName: !!(response.data as User).name
        } : null,
        timestamp: new Date().toISOString()
      };
      
      console.log('✅ [AUTH-CLIENT] Registration SUCCESS:', successDiagnostic);
      
      if (this.debugMode && response.data) {
        console.log('📄 Full Response Data (DEBUG):', JSON.stringify(response.data, null, 2));
      }
      
      console.groupEnd();
      return response.data as User;
      
    } catch (error) {
      const endTime = performance.now();
      const duration = endTime - startTime;
      
      // 🚨 ERROR: 完全なエラー診断情報
      const errorDiagnostic = {
        requestId: diagnosticInfo.requestId,
        status: 'ERROR',
        duration: `${duration.toFixed(2)}ms`,
        error: {
          name: error instanceof Error ? error.name : 'Unknown',
          message: error instanceof Error ? error.message : String(error),
          stack: error instanceof Error ? error.stack?.split('\n').slice(0, 5) : null
        },
        requestInfo: {
          flowId: flowId,
          email: email ? `${email.substring(0, 3)}***@${email.split('@')[1] || 'unknown'}` : 'missing',
          payloadSize: JSON.stringify(payload).length
        },
        timestamp: new Date().toISOString()
      };
      
      console.error('❌ [AUTH-CLIENT] Registration FAILED:', errorDiagnostic);
      
      if (this.debugMode) {
        console.error('📄 Full Error Details (DEBUG):', error);
        console.error('📄 Sent Payload (DEBUG):', JSON.stringify(payload, null, 2));
      }
      
      console.groupEnd();
      throw error;
    }
  }

  async logout(): Promise<void> {
    await this.makeRequest('POST', '/logout');
  }

  async getCurrentUser(): Promise<User | null> {
    try {
      const url = `${this.baseURL}/validate`;
      const response = await fetch(url, {
        method: 'GET',
        credentials: 'include',
      });

      if (response.status === 401) {
        return null; // Not authenticated
      }

      if (!response.ok) {
        throw new Error(this.getMethodDescription('GET', '/validate'));
      }

      const data = await response.json();
      return data.data as User;
    } catch (error: unknown) {
      if (error instanceof Error && error.message && (error.message.includes('401') || error.message.includes('Unauthorized'))) {
        return null; // Not authenticated
      }
      throw error;
    }
  }

  async getCSRFToken(): Promise<string | null> {
    try {
      const response = await this.makeRequest('POST', '/csrf');

      // 防御的プログラミング: CSRF レスポンスの検証強化
      if (!response || !response.data || typeof response.data !== 'object') {
        console.warn('CSRF response invalid format:', response);
        return null;
      }

      const csrfData = response.data as { csrf_token?: string };
      if (!csrfData.csrf_token || typeof csrfData.csrf_token !== 'string') {
        console.warn('CSRF response missing token:', csrfData);
        return null;
      }

      return csrfData.csrf_token;
    } catch (error: unknown) {
      console.warn('Failed to get CSRF token:', error);
      return null;
    }
  }

  async updateProfile(profile: Partial<User>): Promise<User> {
    const response = await this.makeRequest('PUT', '/profile', profile);
    return response.data as User;
  }

  async getUserSettings(): Promise<UserPreferences> {
    const response = await this.makeRequest('GET', '/settings');
    return response.data as UserPreferences;
  }

  async updateUserSettings(settings: UserPreferences): Promise<void> {
    await this.makeRequest('PUT', '/settings', settings);
  }

  // 🔍 ULTRA-DIAGNOSTIC: 緊急診断エンドポイント
  async diagnoseRegistrationFlow(): Promise<any> {
    try {
      console.log('🔍 [AUTH-CLIENT] Starting registration flow diagnosis...');
      const response = await this.makeRequest('GET', '/debug/registration-flow');
      console.log('✅ [AUTH-CLIENT] Registration flow diagnosis completed:', response.data);
      return response.data;
    } catch (error) {
      console.error('❌ [AUTH-CLIENT] Registration flow diagnosis failed:', error);
      throw error;
    }
  }

  // 🔧 Content-Type自動判定システム
  private async determineOptimalContentType(endpoint: string): Promise<string> {
    const cacheKey = `content-type-${endpoint}`;
    const now = Date.now();
    
    // キャッシュチェック（5分間有効）
    if (this.contentTypeCache.has(cacheKey)) {
      const expiry = this.cacheExpiry.get(cacheKey) || 0;
      if (now < expiry) {
        const cachedType = this.contentTypeCache.get(cacheKey)!;
        console.log(`📋 Using cached content-type for ${endpoint}: ${cachedType}`);
        return cachedType;
      }
    }

    // X17.md Phase 17.3: HAR分析により判明 - この修正は不要だった
    // Content-Type判定ロジックを元の適切な実装に戻す
    let optimalContentType: string;
    
    // Kratosエンドポイントの判定 - 実際は適切に動作していた
    if (endpoint.includes('/register/') || endpoint.includes('/login/')) {
      // Kratosフロー完了エンドポイント → JSON形式が適切
      optimalContentType = 'application/json';
    } else {
      // 通常のAPIエンドポイント → JSON使用  
      optimalContentType = 'application/json';
    }
    
    console.log(`📋 Content-Type determined: ${endpoint} → ${optimalContentType}`);

    // キャッシュに保存（5分間）
    this.contentTypeCache.set(cacheKey, optimalContentType);
    this.cacheExpiry.set(cacheKey, now + 5 * 60 * 1000);

    return optimalContentType;
  }

  // X17.md Phase 17.3: 元の適切な実装に戻す - HAR分析で問題なしと判明
  private formatPayloadByContentType(data: any, contentType: string): string | FormData {
    if (contentType === 'application/x-www-form-urlencoded') {
      return this.toURLEncodedString(data);
    } else {
      return JSON.stringify(data);
    }
  }

  // X17.md Phase 17.3: 削除されたtoURLEncodedStringメソッドを復元
  private toURLEncodedString(data: any): string {
    const params = new URLSearchParams();
    
    const flattenObject = (obj: any, prefix = '') => {
      for (const key in obj) {
        if (obj.hasOwnProperty(key)) {
          const value = obj[key];
          const newKey = prefix ? `${prefix}.${key}` : key;
          
          if (value !== null && typeof value === 'object' && !Array.isArray(value)) {
            flattenObject(value, newKey);
          } else if (Array.isArray(value)) {
            value.forEach((item, index) => {
              if (item !== null && typeof item === 'object') {
                flattenObject(item, `${newKey}[${index}]`);
              } else {
                params.append(`${newKey}[${index}]`, String(item));
              }
            });
          } else {
            params.append(newKey, String(value));
          }
        }
      }
    };
    
    flattenObject(data);
    return params.toString();
  }


  // 🎯 Kratosスキーマ完全準拠ペイロード生成システム
  private createKratosCompliantRegistrationPayload(email: string, password: string, name?: string): any {
    console.log('🔧 Creating Kratos-compliant registration payload...');
    
    // Kratosスキーマに完全準拠したペイロード構造
    const payload: any = {
      method: "password",  // Kratosで正確に認識されるmethod
      password: password.trim(),
      traits: {
        email: email.trim().toLowerCase()  // 正規化
      }
    };

    // name処理の改善 - Kratosスキーマに合わせた正確な構造
    if (name && name.trim()) {
      const normalizedName = name.trim();
      const nameParts = normalizedName.split(/\s+/); // 複数の空白を処理
      
      if (nameParts.length >= 1) {
        // Kratosが期待するname構造
        payload.traits.name = {
          first: nameParts[0] || "",
          last: nameParts.slice(1).join(" ") || ""
        };
        
        console.log('🏷️ Name structure created:', {
          original: normalizedName,
          first: payload.traits.name.first,
          last: payload.traits.name.last
        });
      }
    }

    // 空のlastは削除（Kratosで問題を起こす可能性）
    if (payload.traits.name && payload.traits.name.last === "") {
      delete payload.traits.name.last;
    }

    console.log('✅ Kratos-compliant payload created:', {
      hasMethod: !!payload.method,
      hasPassword: !!payload.password,
      hasTraits: !!payload.traits,
      hasEmail: !!payload.traits?.email,
      hasName: !!payload.traits?.name,
      nameStructure: payload.traits?.name || 'none'
    });

    return payload;
  }

  // 🎯 Kratosスキーマ完全準拠ペイロード生成（ログイン用）
  private createKratosCompliantLoginPayload(email: string, password: string): any {
    console.log('🔧 Creating Kratos-compliant login payload...');
    
    // Kratosログイン用スキーマに完全準拠
    const payload = {
      method: "password",
      identifier: email.trim().toLowerCase(),  // Kratosは "identifier" を期待
      password: password.trim()
    };

    console.log('✅ Kratos-compliant login payload created:', {
      hasMethod: !!payload.method,
      hasIdentifier: !!payload.identifier,
      hasPassword: !!payload.password
    });

    return payload;
  }

  // 🔍 ULTRA-DIAGNOSTIC: Kratosレスポンス完全キャプチャ
  async captureKratosResponse(endpoint: string, method: string, payload?: any): Promise<any> {
    const captureId = `CAPTURE-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
    
    console.groupCollapsed(`🎥 [AUTH-CLIENT-CAPTURE] Kratos Response Capture ${captureId}`);
    
    const captureInfo = {
      captureId,
      timestamp: new Date().toISOString(),
      endpoint,
      method,
      payloadProvided: !!payload,
      payloadSize: payload ? JSON.stringify(payload).length : 0,
      userAgent: navigator.userAgent,
      url: window.location.href
    };
    
    console.log('📋 Capture Info:', captureInfo);
    
    if (payload && this.debugMode) {
      console.log('📦 Payload (DEBUG):', JSON.stringify(payload, null, 2));
    }
    
    try {
      const startTime = performance.now();
      const response = await this.makeRequest(method as any, endpoint, payload);
      const endTime = performance.now();
      const duration = endTime - startTime;
      
      const responseAnalysis = {
        captureId,
        status: 'SUCCESS',
        duration: `${duration.toFixed(2)}ms`,
        responseSize: JSON.stringify(response).length,
        hasData: !!response.data,
        responseType: typeof response.data,
        responseKeys: response.data && typeof response.data === 'object' 
          ? Object.keys(response.data) 
          : null,
        timestamp: new Date().toISOString()
      };
      
      console.log('✅ Response Analysis:', responseAnalysis);
      
      if (this.debugMode) {
        console.log('📄 Full Response (DEBUG):', JSON.stringify(response, null, 2));
      }
      
      console.groupEnd();
      return response;
      
    } catch (error) {
      const endTime = performance.now();
      const duration = endTime - performance.now();
      
      const errorAnalysis = {
        captureId,
        status: 'ERROR',
        duration: `${duration.toFixed(2)}ms`,
        errorType: typeof error,
        errorName: error instanceof Error ? error.name : 'Unknown',
        errorMessage: error instanceof Error ? error.message : String(error),
        timestamp: new Date().toISOString()
      };
      
      console.error('❌ Error Analysis:', errorAnalysis);
      
      if (this.debugMode) {
        console.error('🚨 Full Error Details (DEBUG):', error);
      }
      
      console.groupEnd();
      throw error;
    }
  }

  private async makeRequest(method: string, endpoint: string, body?: unknown): Promise<{ data: unknown }> {
    const requestId = `REQ-${++this.requestId}`;
    const startTime = performance.now();
    
    console.log(`🚀 [${requestId}] Starting request: ${method} ${endpoint}`);
    
    const url = `${this.baseURL}${endpoint}`;
    const isUnsafeMethod = ['POST', 'PUT', 'PATCH', 'DELETE'].includes(method.toUpperCase());
    const isCsrfEndpoint = endpoint.includes('/csrf');

    const headers: Record<string, string> = {};

    // Add CSRF token for unsafe methods (but not for CSRF endpoint itself to avoid circular dependency)
    if (isUnsafeMethod && !isCsrfEndpoint) {
      const csrfToken = await this.getCSRFTokenInternal();
      if (csrfToken) {
        headers['X-CSRF-Token'] = csrfToken;
      }
    }

    // 🔧 Content-Type選択とペイロード変換 - JSON固定
    let formattedBody: string | FormData | undefined;
    if (body) {
      const optimalContentType = await this.determineOptimalContentType(endpoint);
      headers['Content-Type'] = optimalContentType;
      formattedBody = this.formatPayloadByContentType(body, optimalContentType);
      
      console.log(`📋 [${requestId}] Content-Type: ${optimalContentType}`);
      if (this.debugMode) {
        console.log(`📦 [${requestId}] Formatted body:`, 
          optimalContentType === 'application/json' 
            ? JSON.parse(formattedBody as string)
            : formattedBody
        );
      }
    }

    const config: RequestInit = {
      method,
      credentials: 'include', // Include cookies
      headers,
    };

    if (formattedBody) {
      config.body = formattedBody;
    }

    try {
      const response = await fetch(url, config);
      const duration = performance.now() - startTime;

      console.log(`🏁 [${requestId}] Request completed: ${response.status} in ${duration.toFixed(2)}ms`);

      if (!response.ok) {
        const errorContext = this.getMethodDescription(method, endpoint);
        const error = new Error(`HTTP ${response.status}: ${errorContext}`);
        console.error(`❌ [${requestId}] Request failed:`, error);
        throw this.handleError(error, errorContext);
      }

      const result = await response.json();
      console.log(`✅ [${requestId}] Request successful`);
      return result;
    } catch (error) {
      const duration = performance.now() - startTime;
      const errorContext = this.getMethodDescription(method, endpoint);
      console.error(`💥 [${requestId}] Request error after ${duration.toFixed(2)}ms:`, error);
      throw this.handleError(error, errorContext);
    }
  }

  private async getCSRFTokenInternal(): Promise<string | null> {
    try {
      const url = `${this.baseURL}/csrf`;
      const response = await fetch(url, {
        method: 'POST',
        credentials: 'include',
      });

      if (!response.ok) {
        return null;
      }

      const data = await response.json();
      return data.data?.csrf_token || null;
    } catch {
      return null;
    }
  }

  // 詳細エラー分析とKratos固有エラー変換
  private handleError(error: unknown, context: string): Error {
    // 詳細診断ログ
    console.error(`Auth API Error [${context}]:`, error);
    console.error(`Auth API Error [${context}] - Type:`, typeof error);

    if (error instanceof Error) {
      console.error(`Auth API Error [${context}] - Message:`, error.message);
      console.error(`Auth API Error [${context}] - Name:`, error.name);

      // Kratos固有エラーの判定と適切な変換
      if (error.message.includes('Property email is missing')) {
        return new Error(`VALIDATION_FAILED: Property email is missing - ${context}`);
      }

      if (error.message.includes('already registered') || error.message.includes('User already exists')) {
        return new Error(`USER_ALREADY_EXISTS: User already exists - ${context}`);
      }

      if (error.message.includes('flow expired') || error.message.includes('410')) {
        return new Error(`FLOW_EXPIRED: Registration flow expired - ${context}`);
      }

      if (error.message.includes('502') || error.message.includes('503')) {
        return new Error(`KRATOS_SERVICE_ERROR: Authentication service unavailable - ${context}`);
      }

      // HTTPステータスコード別の処理
      if (error.message.includes('HTTP 400')) {
        return new Error(`VALIDATION_FAILED: Bad request - ${context}: ${error.message}`);
      }

      // 🚨 FIX: HTTP 401 専用ハンドリング追加
      if (error.message.includes('HTTP 401')) {
        return new Error(`SESSION_NOT_FOUND: Authentication required - ${context}: ${error.message}`);
      }

      // 🚨 FIX: HTTP 404 専用ハンドリング追加
      if (error.message.includes('HTTP 404')) {
        return new Error(`KRATOS_SERVICE_ERROR: Authentication endpoint not found - ${context}: ${error.message}`);
      }

      if (error.message.includes('HTTP 409')) {
        return new Error(`USER_ALREADY_EXISTS: Conflict - ${context}: ${error.message}`);
      }

      if (error.message.includes('HTTP 410')) {
        return new Error(`FLOW_EXPIRED: Gone - ${context}: ${error.message}`);
      }

      return new Error(`${context}: ${error.message}`);
    }

    return new Error(`${context}: Unknown error occurred`);
  }

  private getMethodDescription(method: string, endpoint: string): string {
    if (endpoint.includes('/login') && method === 'POST' && !endpoint.includes('/login/')) {
      return 'Failed to initiate login';
    }
    if (endpoint.includes('/login/') && method === 'POST') {
      return 'Failed to complete login';
    }
    if (endpoint.includes('/register') && method === 'POST' && !endpoint.includes('/register/')) {
      return 'Failed to initiate registration';
    }
    if (endpoint.includes('/register/') && method === 'POST') {
      return 'Failed to complete registration';
    }
    if (endpoint.includes('/logout')) {
      return 'Failed to logout';
    }
    if (endpoint.includes('/validate')) {
      return 'Failed to get current user';
    }
    if (endpoint.includes('/csrf')) {
      return 'Failed to get CSRF token';
    }
    if (endpoint.includes('/profile')) {
      return 'Failed to update profile';
    }
    if (endpoint.includes('/settings')) {
      return method === 'GET' ? 'Failed to get user settings' : 'Failed to update user settings';
    }
    return `Request failed`;
  }
}

// Export singleton instance
export const authAPI = new AuthAPIClient();