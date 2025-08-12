import React, { createContext, useContext, useEffect, useState, ReactNode, useCallback } from 'react';
import { authAPI } from '@/lib/api/auth-client';
import type { AuthState } from '@/types/auth';

// エラータイプの定義
export type AuthErrorType = 
  | 'NETWORK_ERROR'
  | 'INVALID_CREDENTIALS'
  | 'REGISTRATION_FAILED'
  | 'SESSION_EXPIRED'
  | 'VALIDATION_ERROR'
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

// エラーマッピング関数
const mapErrorToAuthError = (error: unknown, retryCount = 0): AuthError => {
  if (error instanceof Error) {
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
    
    // 登録エラーの検出
    if (error.message.includes('registration') || error.message.includes('User already exists')) {
      return {
        type: 'REGISTRATION_FAILED',
        message: 'アカウント作成に失敗しました。すでに登録されているメールアドレスの可能性があります',
        isRetryable: false,
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
      
      // Initiate login flow
      const loginFlow = await authAPI.initiateLogin();
      
      // Complete login with credentials
      const user = await authAPI.completeLogin(loginFlow.id, email, password);
      
      setAuthState(prev => ({
        ...prev,
        user,
        isAuthenticated: true,
        isLoading: false,
        error: null,
        lastActivity: new Date(),
      }));
      
      // ログイン成功時は前回のアクションをクリア
      setLastAction(null);
    } catch (error: unknown) {
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
      
      // Initiate registration flow
      const registrationFlow = await authAPI.initiateRegistration();
      
      // Complete registration with user data
      const user = await authAPI.completeRegistration(registrationFlow.id, email, password, name);
      
      setAuthState(prev => ({
        ...prev,
        user,
        isAuthenticated: true,
        isLoading: false,
        error: null,
        lastActivity: new Date(),
      }));
      
      // 登録成功時は前回のアクションをクリア
      setLastAction(null);
    } catch (error: unknown) {
      const authError = mapErrorToAuthError(error);
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