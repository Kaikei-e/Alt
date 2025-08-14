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
}

interface ExtendedAuthState extends Omit<AuthState, 'error'> {
  error: AuthError | null;
  lastActivity: Date | null;
  sessionTimeout: number; // minutes
}

interface AuthContextType extends ExtendedAuthState {
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, name?: string) => Promise<void>;
  logout: () => Promise<void>;
  refresh: () => Promise<void>;
  clearError: () => void;
  retryLastAction: () => Promise<void>;
}

const AuthContext = createContext<AuthContextType | null>(null);

interface AuthProviderProps {
  children: ReactNode;
}

// エラーマッピング関数 - 詳細診断ログ付き
const mapErrorToAuthError = (error: unknown, retryCount = 0): AuthError => {
  // 詳細診断ログ
  console.log('[AUTH-CONTEXT] Error mapping - Input error:', error);
  console.log('[AUTH-CONTEXT] Error mapping - Error type:', typeof error);
  console.log('[AUTH-CONTEXT] Error mapping - Retry count:', retryCount);

  if (error instanceof Error) {
    console.log('[AUTH-CONTEXT] Error mapping - Error message:', error.message);
    console.log('[AUTH-CONTEXT] Error mapping - Error name:', error.name);
    // ネットワークエラーの検出
    if (error.message.includes('Failed to fetch') || error.message.includes('Network request failed')) {
      return {
        type: 'NETWORK_ERROR',
        message: 'ネットワーク接続を確認してください',
        isRetryable: true,
        retryCount
      };
    }

    // 認証エラーの検出
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

    // 精密なエラー分類 - 既存ユーザーの明確な検出
    if (error.message.includes('User already exists') ||
        error.message.includes('already registered') ||
        error.message.includes('email already taken') ||
        error.message.includes('409')) {
      return {
        type: 'USER_ALREADY_EXISTS',
        message: 'このメールアドレスは既に登録されています。ログインをお試しください',
        isRetryable: false,
        retryCount
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

    return {
      type: 'UNKNOWN_ERROR',
      message: error.message || '予期しないエラーが発生しました',
      isRetryable: true,
      retryCount
    };
  }

  return {
    type: 'UNKNOWN_ERROR',
    message: '予期しないエラーが発生しました',
    isRetryable: true,
    retryCount
  };
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

  // アクティビティ更新
  const updateActivity = useCallback(() => {
    if (authState.isAuthenticated) {
      setAuthState(prev => ({ ...prev, lastActivity: new Date() }));
    }
  }, [authState.isAuthenticated]);

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

      // Initiate login flow with validation
      const loginFlow = await authAPI.initiateLogin();

      // 🚨 防御的プログラミング: flow オブジェクト検証強化
      if (!loginFlow || !loginFlow.id) {
        throw new Error('Login flow initialization failed: missing flow ID');
      }

      console.log('[AUTH-CONTEXT] Login flow initialized:', { flowId: loginFlow.id, timestamp: new Date().toISOString() });

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

      // Initiate registration flow with validation
      const registrationFlow = await authAPI.initiateRegistration();

      // 🚨 防御的プログラミング: flow オブジェクト検証強化
      if (!registrationFlow || !registrationFlow.id) {
        throw new Error('Registration flow initialization failed: missing flow ID');
      }

      console.log('[AUTH-CONTEXT] Registration flow initialized:', { flowId: registrationFlow.id, timestamp: new Date().toISOString() });

      // Complete registration with user data
      const user = await authAPI.completeRegistration(registrationFlow.id, email, password, name);

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

  const contextValue: AuthContextType = {
    ...authState,
    login,
    register,
    logout,
    refresh,
    clearError,
    retryLastAction,
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