import React, { createContext, useContext, useEffect, useState, ReactNode, useCallback } from 'react';
import { authAPI } from '@/lib/api/auth-client';
import type { AuthState } from '@/types/auth';

// エラータイプの定義 - 精密なエラー分類
export type AuthErrorType =
  | 'NETWORK_ERROR'
  | 'INVALID_CREDENTIALS'
  | 'USER_ALREADY_EXISTS'          // 新規: 既存ユーザー専用
  | 'REGISTRATION_FAILED'          // 汎用的な登録エラー
  | 'SESSION_EXPIRED'
  | 'SESSION_NOT_FOUND'            // 新規: セッションが見つからない（401エラー）
  | 'VALIDATION_ERROR'
  | 'FLOW_EXPIRED'                 // 新規: フロー期限切れ
  | 'KRATOS_SERVICE_ERROR'         // 新規: Kratosサービスエラー
  | 'DATA_FORMAT_ERROR'            // 新規: データ形式エラー
  | 'UNKNOWN_ERROR'
  | 'TIMEOUT_ERROR';

export interface AuthError {
  type: AuthErrorType;
  message: string;
  isRetryable: boolean;
  retryCount?: number;
  // 🔄 Phase 4: 詳細エラー情報
  technicalInfo?: string;
  errorCode?: string;
  suggestions?: string[];
  retryAfter?: number;
}

interface ExtendedAuthState extends Omit<AuthState, 'error'> {
  error: AuthError | null;
  lastActivity: Date | null;
  sessionTimeout: number; // minutes
}

// 🔄 Phase 3: フロー管理状態
interface FlowState {
  registrationFlow: any | null;
  loginFlow: any | null;
  expiresAt: Date | null;
  isExpired: boolean;
  lastRefreshTime: Date | null;
}

interface AuthContextType extends ExtendedAuthState {
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name?: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  clearError: () => void;
  retryLastAction: () => Promise<void>;
  // 🔍 ULTRA-DIAGNOSTIC: 開発者向けデバッグ機能
  debugDiagnoseRegistrationFlow: () => Promise<any>;
  debugCaptureNextRequest: (enable: boolean) => void;
  // 🔄 Phase 3: フロー管理機能
  ensureValidRegistrationFlow: () => Promise<any>;
  ensureValidLoginFlow: () => Promise<any>;
  isFlowValid: (flow: any) => boolean;
}

const AuthContext = createContext<AuthContextType | null>(null);

interface AuthProviderProps {
  children: ReactNode;
}

// エラーマッピング関数 - 詳細診断ログ付き
const mapErrorToAuthError = (error: unknown, retryCount = 0): AuthError => {
  // 詳細診断ログ
  console.groupCollapsed('[AUTH-CONTEXT] 🔍 Error Mapping Analysis');
  console.log('Input error:', error);
  console.log('Error type:', typeof error);
  console.log('Retry count:', retryCount);

  if (error instanceof Error) {
    console.log('Error message:', error.message);
    console.log('Error name:', error.name);

    // 🔄 Phase 4: バックエンドからの詳細エラー情報を抽出
    const extractDetailedErrorInfo = (errorMessage: string) => {
      // "[ERROR_TYPE]: message" パターンをチェック
      const detailedErrorMatch = errorMessage.match(/\[([A-Z_]+)\]: (.+)/);
      if (detailedErrorMatch) {
        const [, errorType, message] = detailedErrorMatch;
        console.log('🎯 Detailed error detected:', { errorType, message });
        return { errorType, message };
      }
      return null;
    };

    const detailedInfo = extractDetailedErrorInfo(error.message);

    // 🔄 Phase 4: 詳細エラー情報がある場合の処理
    if (detailedInfo) {
      console.log('✅ Using detailed error info for mapping');
      const baseError: AuthError = {
        type: detailedInfo.errorType as AuthErrorType,
        message: detailedInfo.message,
        isRetryable: true, // デフォルト値、後で調整
        retryCount,
        technicalInfo: `Backend error: ${detailedInfo.errorType}`,
        errorCode: detailedInfo.errorType,
      };

      // 詳細エラータイプ別の調整
      switch (detailedInfo.errorType) {
        case 'MISSING_EMAIL_FIELD':
          baseError.type = 'DATA_FORMAT_ERROR';
          baseError.isRetryable = true;
          baseError.suggestions = [
            'メールアドレスフィールドが正しく送信されているか確認してください',
            'フォームを再読み込みして再試行してください',
          ];
          break;
        case 'USER_ALREADY_EXISTS':
          baseError.type = 'USER_ALREADY_EXISTS';
          baseError.isRetryable = false;
          baseError.suggestions = [
            '別のメールアドレスを使用してください',
            '既にアカウントをお持ちの場合はログインしてください',
          ];
          break;
        case 'FLOW_EXPIRED':
          baseError.type = 'FLOW_EXPIRED';
          baseError.isRetryable = true;
          baseError.suggestions = [
            'ページを再読み込みして新しい登録フローを開始してください',
          ];
          break;
        case 'SESSION_NOT_FOUND':
          baseError.type = 'SESSION_NOT_FOUND';
          baseError.isRetryable = true;
          baseError.suggestions = [
            '認証が必要です。ログインしてください',
            'ページを再読み込みしてください',
          ];
          break;
        default:
          baseError.isRetryable = true;
      }

      console.groupEnd();
      return baseError;
    }

    // 🚨 FIX: 404 エラーの正確な処理（認証サービス利用不可）
    if (error.message.includes('404') || error.message.includes('Not Found')) {
      return {
        type: 'KRATOS_SERVICE_ERROR',
        message: '認証サービスに接続できません。しばらく後にもう一度お試しください',
        isRetryable: true,
        retryCount,
        technicalInfo: 'Authentication service endpoints not accessible',
        suggestions: ['しばらく待ってから再試行してください', 'サポートにお問い合わせください']
      };
    }

    // ネットワークエラーの検出
    if (error.message.includes('Failed to fetch') || error.message.includes('Network request failed')) {
      const networkError: AuthError = {
        type: 'NETWORK_ERROR',
        message: 'ネットワーク接続を確認してください',
        isRetryable: true,
        retryCount,
        technicalInfo: 'Network connectivity issue',
        suggestions: ['インターネット接続を確認してください', '再試行してください'],
      };
      console.groupEnd();
      return networkError;
    }

    // 認証エラーの検出 - より精密な分類
    if (error.message.includes('SESSION_NOT_FOUND') || error.message.includes('Authentication required')) {
      return {
        type: 'SESSION_NOT_FOUND',
        message: '認証が必要です。ログインしてください',
        isRetryable: true,
        retryCount,
        technicalInfo: 'Session not found - authentication required',
        suggestions: ['ログインページに移動してください', 'ページを再読み込みしてください']
      };
    }

    if (error.message.includes('401') || error.message.includes('Unauthorized') || error.message.includes('Invalid credentials')) {
      return {
        type: 'INVALID_CREDENTIALS',
        message: 'メールアドレスまたはパスワードが正しくありません',
        isRetryable: false,
        retryCount
      };
    }

    // セッション期限切れの検出
    if (error.message.includes('Session expired') || error.message.includes('Token expired')) {
      return {
        type: 'SESSION_EXPIRED',
        message: 'セッションの有効期限が切れました。再度ログインしてください',
        isRetryable: false,
        retryCount
      };
    }

    // タイムアウトエラーの検出
    if (error.message.includes('timeout') || error.message.includes('AbortError')) {
      return {
        type: 'TIMEOUT_ERROR',
        message: 'リクエストがタイムアウトしました',
        isRetryable: true,
        retryCount
      };
    }

    // 🚨 FIX: より厳密な既存ユーザー検出 - HTTP 409 Conflict 専用
    if ((error.message.includes('409') && 
         (error.message.includes('User already exists') || 
          error.message.includes('already registered') || 
          error.message.includes('email already taken'))) ||
        error.message.includes('USER_ALREADY_EXISTS')) {
      return {
        type: 'USER_ALREADY_EXISTS',
        message: 'このメールアドレスは既に登録されています。ログインをお試しください',
        isRetryable: false,
        retryCount,
        technicalInfo: 'User conflict detected from authentication service'
      };
    }

    // データ形式エラーの明確な分離
    if (error.message.includes('Property email is missing') ||
        error.message.includes('missing properties') ||
        error.message.includes('traits') ||
        error.message.includes('VALIDATION_FAILED')) {
      return {
        type: 'DATA_FORMAT_ERROR',
        message: '登録情報の形式に問題があります。メールアドレスとパスワードを確認してください',
        isRetryable: true,
        retryCount
      };
    }

    // フロー期限切れの検出
    if (error.message.includes('flow expired') ||
        error.message.includes('Flow expired') ||
        error.message.includes('410')) {
      return {
        type: 'FLOW_EXPIRED',
        message: '登録フローの有効期限が切れました。最初からやり直してください',
        isRetryable: true,
        retryCount
      };
    }

    // Kratosサービス固有エラー
    if (error.message.includes('kratos') ||
        error.message.includes('Kratos') ||
        error.message.includes('502') ||
        error.message.includes('503')) {
      return {
        type: 'KRATOS_SERVICE_ERROR',
        message: '認証サービスに一時的な問題が発生しています。しばらく後にもう一度お試しください',
        isRetryable: true,
        retryCount
      };
    }

    // 最後の手段として汎用的な登録エラー（より限定的な条件）
    if (error.message.includes('registration failed') ||
        error.message.includes('Registration failed')) {
      return {
        type: 'REGISTRATION_FAILED',
        message: '登録処理中にエラーが発生しました。入力内容を確認してもう一度お試しください',
        isRetryable: true,
        retryCount
      };
    }

    // バリデーションエラーの検出
    if (error.message.includes('validation') || error.message.includes('invalid format')) {
      return {
        type: 'VALIDATION_ERROR',
        message: '入力内容を確認してください',
        isRetryable: false,
        retryCount
      };
    }

    const mappedError = {
      type: 'UNKNOWN_ERROR' as AuthErrorType,
      message: error.message || '予期しないエラーが発生しました',
      isRetryable: true,
      retryCount
    };
    
    console.log('🎯 Final Mapped Error:', mappedError);
    console.groupEnd();
    return mappedError;
  }

  const mappedError = {
    type: 'UNKNOWN_ERROR' as AuthErrorType,
    message: '予期しないエラーが発生しました',
    isRetryable: true,
    retryCount
  };
  
  console.log('🎯 Final Mapped Error:', mappedError);
  console.groupEnd();
  return mappedError;
};

export function AuthProvider({ children }: AuthProviderProps) {
  const [authState, setAuthState] = useState<ExtendedAuthState>({
    user: null,
    isAuthenticated: false,
    isLoading: true,
    error: null,
    lastActivity: null,
    sessionTimeout: 30, // 30分
  });

  // 最後に実行しようとしたアクション
  const [lastAction, setLastAction] = useState<{
    type: 'login' | 'register' | 'refresh';
    params: any[];
  } | null>(null);

  // 🔍 ULTRA-DIAGNOSTIC: デバッグ状態管理
  const [debugCaptureEnabled, setDebugCaptureEnabled] = useState(false);

  // 🔄 Phase 3: フロー管理状態管理
  const [flowState, setFlowState] = useState<FlowState>({
    registrationFlow: null,
    loginFlow: null,
    expiresAt: null,
    isExpired: false,
    lastRefreshTime: null
  });

  // 自動セッション確認
  useEffect(() => {
    checkAuthStatus();
  }, []);

  // セッション期限監視
  useEffect(() => {
    if (authState.isAuthenticated && authState.lastActivity) {
      const checkInterval = setInterval(() => {
        const now = new Date();
        const lastActivity = authState.lastActivity!;
        const minutesSinceLastActivity = Math.floor((now.getTime() - lastActivity.getTime()) / (1000 * 60));

        if (minutesSinceLastActivity >= authState.sessionTimeout) {
          logout();
        }
      }, 60000); // 1分毎にチェック

      return () => clearInterval(checkInterval);
    }
  }, [authState.isAuthenticated, authState.lastActivity, authState.sessionTimeout]);

  // 🔄 Phase 3: フロー期限監視システム
  useEffect(() => {
    const checkFlowExpiration = () => {
      const now = new Date();
      
      // 登録フロー期限チェック
      if (flowState.registrationFlow && flowState.expiresAt) {
        const timeToExpiry = flowState.expiresAt.getTime() - now.getTime();
        const isExpiring = timeToExpiry < 5 * 60 * 1000; // 5分以内に期限切れ
        
        if (isExpiring && !flowState.isExpired) {
          console.warn('🔄 [FLOW-MANAGER] Registration flow expiring soon:', {
            flowId: flowState.registrationFlow.id,
            expiresAt: flowState.expiresAt.toISOString(),
            timeToExpiry: `${Math.round(timeToExpiry / 1000)}s`
          });
          
          setFlowState(prev => ({ ...prev, isExpired: true }));
        }
      }
      
      // ログインフロー期限チェック
      if (flowState.loginFlow && flowState.expiresAt) {
        const timeToExpiry = flowState.expiresAt.getTime() - now.getTime();
        const isExpiring = timeToExpiry < 5 * 60 * 1000; // 5分以内に期限切れ
        
        if (isExpiring && !flowState.isExpired) {
          console.warn('🔄 [FLOW-MANAGER] Login flow expiring soon:', {
            flowId: flowState.loginFlow.id,
            expiresAt: flowState.expiresAt.toISOString(),
            timeToExpiry: `${Math.round(timeToExpiry / 1000)}s`
          });
          
          setFlowState(prev => ({ ...prev, isExpired: true }));
        }
      }
    };

    // 30秒毎にフロー期限をチェック
    const flowCheckInterval = setInterval(checkFlowExpiration, 30000);

    return () => clearInterval(flowCheckInterval);
  }, [flowState.registrationFlow, flowState.loginFlow, flowState.expiresAt, flowState.isExpired]);

  // アクティビティ更新
  const updateActivity = useCallback(() => {
    if (authState.isAuthenticated) {
      setAuthState(prev => ({ ...prev, lastActivity: new Date() }));
    }
  }, [authState.isAuthenticated]);

  // 🔄 Phase 3: フロー有効性チェック
  const isFlowValid = useCallback((flow: any): boolean => {
    if (!flow || !flow.expiresAt) {
      console.log('🔍 [FLOW-MANAGER] Flow invalid: missing flow or expiresAt', { flow: !!flow, expiresAt: flow?.expiresAt });
      return false;
    }
    
    const now = new Date();
    const expiresAt = new Date(flow.expiresAt);
    const isValid = expiresAt > now;
    
    console.log('🔍 [FLOW-MANAGER] Flow validity check:', {
      flowId: flow.id,
      expiresAt: expiresAt.toISOString(),
      now: now.toISOString(),
      isValid,
      timeToExpiry: `${Math.round((expiresAt.getTime() - now.getTime()) / 1000)}s`
    });
    
    return isValid;
  }, []);

  // 🔄 Phase 3: 有効な登録フロー確保
  const ensureValidRegistrationFlow = useCallback(async (): Promise<any> => {
    const flowManagerId = `REG-FLOW-${Date.now()}`;
    console.log(`🔄 [FLOW-MANAGER] Ensuring valid registration flow - ${flowManagerId}`);
    
    // 既存フローの有効性チェック
    if (isFlowValid(flowState.registrationFlow)) {
      console.log(`✅ [FLOW-MANAGER] Current registration flow is valid - ${flowManagerId}`, {
        flowId: flowState.registrationFlow.id,
        expiresAt: flowState.registrationFlow.expiresAt
      });
      return flowState.registrationFlow;
    }

    console.log(`🔄 [FLOW-MANAGER] Registration flow expired or invalid, regenerating... - ${flowManagerId}`);
    
    try {
      const newFlow = await authAPI.initiateRegistration();
      
      setFlowState(prev => ({
        ...prev,
        registrationFlow: newFlow,
        expiresAt: new Date(newFlow.expiresAt),
        isExpired: false,
        lastRefreshTime: new Date()
      }));

      console.log(`✅ [FLOW-MANAGER] New registration flow created - ${flowManagerId}`, {
        flowId: newFlow.id,
        expiresAt: newFlow.expiresAt,
        timeToExpiry: `${Math.round((new Date(newFlow.expiresAt).getTime() - Date.now()) / 1000)}s`
      });

      return newFlow;
    } catch (error) {
      console.error(`❌ [FLOW-MANAGER] Failed to create registration flow - ${flowManagerId}`, error);
      throw error;
    }
  }, [flowState.registrationFlow, isFlowValid]);

  // 🔄 Phase 3: 有効なログインフロー確保
  const ensureValidLoginFlow = useCallback(async (): Promise<any> => {
    const flowManagerId = `LOGIN-FLOW-${Date.now()}`;
    console.log(`🔄 [FLOW-MANAGER] Ensuring valid login flow - ${flowManagerId}`);
    
    // 既存フローの有効性チェック
    if (isFlowValid(flowState.loginFlow)) {
      console.log(`✅ [FLOW-MANAGER] Current login flow is valid - ${flowManagerId}`, {
        flowId: flowState.loginFlow.id,
        expiresAt: flowState.loginFlow.expiresAt
      });
      return flowState.loginFlow;
    }

    console.log(`🔄 [FLOW-MANAGER] Login flow expired or invalid, regenerating... - ${flowManagerId}`);
    
    try {
      const newFlow = await authAPI.initiateLogin();
      
      setFlowState(prev => ({
        ...prev,
        loginFlow: newFlow,
        expiresAt: new Date(newFlow.expiresAt),
        isExpired: false,
        lastRefreshTime: new Date()
      }));

      console.log(`✅ [FLOW-MANAGER] New login flow created - ${flowManagerId}`, {
        flowId: newFlow.id,
        expiresAt: newFlow.expiresAt,
        timeToExpiry: `${Math.round((new Date(newFlow.expiresAt).getTime() - Date.now()) / 1000)}s`
      });

      return newFlow;
    } catch (error) {
      console.error(`❌ [FLOW-MANAGER] Failed to create login flow - ${flowManagerId}`, error);
      throw error;
    }
  }, [flowState.loginFlow, isFlowValid]);

  // 再試行付きのチェック認証
  const checkAuthStatus = async (retryCount = 0): Promise<void> => {
    try {
      setAuthState(prev => ({ ...prev, isLoading: true, error: null }));
      const user = await authAPI.getCurrentUser();
      setAuthState(prev => ({
        ...prev,
        user,
        isAuthenticated: !!user,
        isLoading: false,
        error: null,
        lastActivity: user ? new Date() : null,
      }));
    } catch (error: unknown) {
      const authError = mapErrorToAuthError(error, retryCount);

      // Enhanced 401 Unauthorized handling - redirect to login (2025 best practice)
      const is401Error = authError.type === 'INVALID_CREDENTIALS' ||
                        (error instanceof Error &&
                         (error.message.includes('401') || error.message.includes('Unauthorized')));

      if (is401Error && typeof window !== 'undefined') {
        console.warn('[AUTH-CONTEXT] 401/Unauthorized detected in checkAuthStatus, redirecting to login');

        // Session expired or invalid, redirect to login with current URL
        const currentUrl = window.location.pathname + window.location.search;
        const returnUrl = encodeURIComponent(currentUrl);
        const loginUrl = `/login?returnUrl=${returnUrl}`;

        console.log('[AUTH-CONTEXT] Redirecting to login:', loginUrl);

        // Use replace for cleaner navigation history
        window.location.replace(loginUrl);
        return;
      }

      // 再試行可能なエラーで再試行回数が3回未満の場合は再試行
      if (authError.isRetryable && retryCount < 3) {
        setTimeout(() => {
          checkAuthStatus(retryCount + 1);
        }, Math.pow(2, retryCount) * 1000); // 指数バックオフ
        return;
      }

      setAuthState(prev => ({
        ...prev,
        user: null,
        isAuthenticated: false,
        isLoading: false,
        error: authError,
        lastActivity: null,
      }));
    }
  };

  const login = async (email: string, password: string) => {
    setLastAction({ type: 'login', params: [email, password] });

    try {
      setAuthState(prev => ({ ...prev, isLoading: true, error: null }));

      // 🔄 Phase 3: 有効なログインフロー確保
      const loginFlow = await ensureValidLoginFlow();

      // 🚨 防御的プログラミング: flow オブジェクト検証強化
      if (!loginFlow || !loginFlow.id) {
        throw new Error('Login flow initialization failed: missing flow ID');
      }

      console.log('[AUTH-CONTEXT] Using valid login flow:', { 
        flowId: loginFlow.id, 
        expiresAt: loginFlow.expiresAt,
        timestamp: new Date().toISOString() 
      });

      // Complete login with credentials
      const user = await authAPI.completeLogin(loginFlow.id, email, password);

      // 🚨 防御的プログラミング: user オブジェクト検証
      if (!user) {
        throw new Error('Login completed but user data is missing');
      }

      setAuthState(prev => ({
        ...prev,
        user,
        isAuthenticated: true,
        isLoading: false,
        error: null,
        lastActivity: new Date(),
      }));

      console.log('[AUTH-CONTEXT] Login successful:', { userId: user.id, timestamp: new Date().toISOString() });

      // ログイン成功時は前回のアクションをクリア
      setLastAction(null);
    } catch (error: unknown) {
      console.error('[AUTH-CONTEXT] Login failed:', error);
      const authError = mapErrorToAuthError(error);
      setAuthState(prev => ({
        ...prev,
        isLoading: false,
        error: authError,
      }));
      throw error;
    }
  };

  const register = async (email: string, password: string, name?: string) => {
    setLastAction({ type: 'register', params: [email, password, name] });

    try {
      setAuthState(prev => ({ ...prev, isLoading: true, error: null }));

      // 🔄 Phase 3: 有効な登録フロー確保
      const registrationFlow = await ensureValidRegistrationFlow();

      // 🚨 防御的プログラミング: flow オブジェクト検証強化
      if (!registrationFlow || !registrationFlow.id) {
        throw new Error('Registration flow initialization failed: missing flow ID');
      }

      console.log('[AUTH-CONTEXT] Using valid registration flow:', { 
        flowId: registrationFlow.id, 
        expiresAt: registrationFlow.expiresAt,
        timestamp: new Date().toISOString() 
      });

      // Complete registration with user data
      const user = debugCaptureEnabled 
        ? await authAPI.captureKratosResponse(`/register/${registrationFlow.id}`, 'POST', {
            traits: {
              email: email.trim(),
              name: name ? {
                first: name.split(' ')[0]?.trim() || '',
                last: name.split(' ').slice(1).join(' ')?.trim() || ''
              } : undefined
            },
            password: password,
            method: 'profile'
          }).then(response => response.data as any)
        : await authAPI.completeRegistration(registrationFlow.id, email, password, name);

      // 🚨 防御的プログラミング: user オブジェクト検証
      if (!user) {
        throw new Error('Registration completed but user data is missing');
      }

      setAuthState(prev => ({
        ...prev,
        user,
        isAuthenticated: true,
        isLoading: false,
        error: null,
        lastActivity: new Date(),
      }));

      console.log('[AUTH-CONTEXT] Registration successful:', { userId: user.id, timestamp: new Date().toISOString() });

      // 登録成功時は前回のアクションをクリア
      setLastAction(null);
    } catch (error: unknown) {
      // 詳細ログ出力でデバッグ性向上
      console.error('[AUTH-CONTEXT] Registration failed - Raw error:', error);
      console.error('[AUTH-CONTEXT] Registration failed - Error type:', typeof error);
      console.error('[AUTH-CONTEXT] Registration failed - Flow ID:', 'flow_id_not_available');
      console.error('[AUTH-CONTEXT] Registration failed - Email:', email ? 'provided' : 'missing');
      console.error('[AUTH-CONTEXT] Registration failed - Password:', password ? 'provided' : 'missing');
      console.error('[AUTH-CONTEXT] Registration failed - Name:', name || 'not provided');

      if (error instanceof Error) {
        console.error('[AUTH-CONTEXT] Registration failed - Error message:', error.message);
        console.error('[AUTH-CONTEXT] Registration failed - Error stack:', error.stack);
      }

      const authError = mapErrorToAuthError(error);
      console.error('[AUTH-CONTEXT] Registration failed - Mapped error type:', authError.type);
      console.error('[AUTH-CONTEXT] Registration failed - Mapped error message:', authError.message);
      console.error('[AUTH-CONTEXT] Registration failed - Is retryable:', authError.isRetryable);

      setAuthState(prev => ({
        ...prev,
        isLoading: false,
        error: authError,
      }));
      throw error;
    }
  };

  const logout = async () => {
    try {
      setAuthState(prev => ({ ...prev, error: null }));
      await authAPI.logout();
      setAuthState(prev => ({
        ...prev,
        user: null,
        isAuthenticated: false,
        isLoading: false,
        error: null,
        lastActivity: null,
      }));
      setLastAction(null); // ログアウト時はアクション履歴もクリア
    } catch (error: unknown) {
      // ログアウトエラーは重要ではないのでローカル状態をクリア
      setAuthState(prev => ({
        ...prev,
        user: null,
        isAuthenticated: false,
        isLoading: false,
        error: null,
        lastActivity: null,
      }));
      console.warn('Logout API failed, but local state cleared:', error);
    }
  };

  const refresh = async () => {
    setLastAction({ type: 'refresh', params: [] });
    await checkAuthStatus();
  };

  const retryLastAction = async () => {
    if (!lastAction) {
      throw new Error('再試行可能なアクションがありません');
    }

    const { type, params } = lastAction;

    try {
      switch (type) {
        case 'login':
          await login(params[0], params[1]);
          break;
        case 'register':
          await register(params[0], params[1], params[2]);
          break;
        case 'refresh':
          await refresh();
          break;
        default:
          throw new Error('不明なアクションタイプです');
      }
    } catch (error) {
      // エラーは元の関数で処理されるため、ここでは再スロー
      throw error;
    }
  };

  const clearError = () => {
    setAuthState(prev => ({ ...prev, error: null }));
  };

  // 🔍 ULTRA-DIAGNOSTIC: 開発者向け診断機能
  const debugDiagnoseRegistrationFlow = async (): Promise<any> => {
    const diagnosticId = `DIAG-CTX-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;
    
    console.groupCollapsed(`🔬 [AUTH-CONTEXT-DIAGNOSTIC] Full System Diagnosis ${diagnosticId}`);
    console.log('🚀 Starting comprehensive registration flow diagnosis...');
    
    try {
      // システム状態の診断
      const systemState = {
        diagnosticId,
        timestamp: new Date().toISOString(),
        authState: {
          isAuthenticated: authState.isAuthenticated,
          isLoading: authState.isLoading,
          hasUser: !!authState.user,
          hasError: !!authState.error,
          errorType: authState.error?.type || null,
          lastActivity: authState.lastActivity?.toISOString() || null
        },
        browserState: {
          userAgent: navigator.userAgent,
          cookieEnabled: navigator.cookieEnabled,
          onLine: navigator.onLine,
          language: navigator.language,
          currentUrl: window.location.href,
          sessionStorageKeys: Object.keys(sessionStorage),
          localStorageKeys: Object.keys(localStorage),
          documentCookies: document.cookie.split(';').length
        },
        lastAction: lastAction || null
      };
      
      console.log('📊 Current System State:', systemState);
      
      // バックエンド診断の実行
      const backendDiagnostic = await authAPI.diagnoseRegistrationFlow();
      console.log('🔧 Backend Diagnostic Results:', backendDiagnostic);
      
      // 統合診断結果
      const fullDiagnostic = {
        diagnosticId,
        timestamp: new Date().toISOString(),
        frontend: systemState,
        backend: backendDiagnostic,
        recommendations: generateDiagnosticRecommendations(systemState, backendDiagnostic)
      };
      
      console.log('🎯 Complete Diagnostic Results:', fullDiagnostic);
      console.groupEnd();
      
      return fullDiagnostic;
      
    } catch (error) {
      console.error('❌ Diagnostic failed:', error);
      console.groupEnd();
      throw error;
    }
  };

  const debugCaptureNextRequest = (enable: boolean) => {
    setDebugCaptureEnabled(enable);
    console.log(`🎥 Request capture ${enable ? 'ENABLED' : 'DISABLED'}`);
    
    if (enable) {
      console.log('🔍 Next registration request will be fully captured');
      console.log('💡 Use authAPI.captureKratosResponse() directly for manual capture');
    }
  };

  // 診断結果に基づく推奨事項生成
  const generateDiagnosticRecommendations = (frontendState: any, backendDiagnostic: any): string[] => {
    const recommendations: string[] = [];

    // フロントエンドの状態チェック
    if (frontendState.authState.hasError) {
      recommendations.push(`🔧 現在のエラー "${frontendState.authState.errorType}" を確認してください`);
    }

    if (!frontendState.browserState.cookieEnabled) {
      recommendations.push('🍪 ブラウザのクッキーが無効になっています。有効にしてください');
    }

    if (!frontendState.browserState.onLine) {
      recommendations.push('🌐 ネットワーク接続を確認してください');
    }

    // バックエンドの状態チェック
    if (backendDiagnostic?.kratosStatus?.isConnected === false) {
      recommendations.push('🔌 Kratos認証サービスへの接続に問題があります');
    }

    if (backendDiagnostic?.flowTest?.testStatus === 'PARTIAL_FAILURE') {
      recommendations.push('⚠️ 登録フローテストで部分的な失敗が検出されました');
    }

    if (backendDiagnostic?.databaseTest?.isConnected === false) {
      recommendations.push('🗃️ データベース接続に問題があります');
    }

    if (recommendations.length === 0) {
      recommendations.push('✅ システムは正常に動作しているようです');
      recommendations.push('💡 実際の登録試行時のログを確認してください');
    }

    return recommendations;
  };

  const contextValue: AuthContextType = {
    ...authState,
    login,
    register,
    logout,
    refresh,
    clearError,
    retryLastAction,
    debugDiagnoseRegistrationFlow,
    debugCaptureNextRequest,
    // 🔄 Phase 3: フロー管理機能
    ensureValidRegistrationFlow,
    ensureValidLoginFlow,
    isFlowValid,
  };

  return (
    <AuthContext.Provider value={contextValue}>
      {children}
    </AuthContext.Provider>
  );
}

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error('useAuth must be used within an AuthProvider');
  }
  return context;
}