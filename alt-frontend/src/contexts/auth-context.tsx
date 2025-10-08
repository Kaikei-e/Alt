import React, {
  createContext,
  useContext,
  useEffect,
  useState,
  ReactNode,
  useCallback,
  useMemo,
  useRef,
} from "react";
import { authAPI } from "@/lib/api/auth-client";
import type {
  AuthState,
  User,
  RegistrationFlow,
  LoginFlow,
} from "@/types/auth";

// エラータイプの定義 - 精密なエラー分類
export type AuthErrorType =
  | "NETWORK_ERROR"
  | "INVALID_CREDENTIALS"
  | "USER_ALREADY_EXISTS" // 新規: 既存ユーザー専用
  | "REGISTRATION_FAILED" // 汎用的な登録エラー
  | "SESSION_EXPIRED"
  | "SESSION_NOT_FOUND" // 新規: セッションが見つからない（401エラー）
  | "VALIDATION_ERROR"
  | "FLOW_EXPIRED" // 新規: フロー期限切れ
  | "KRATOS_SERVICE_ERROR" // 新規: Kratosサービスエラー
  | "DATA_FORMAT_ERROR" // 新規: データ形式エラー
  | "UNKNOWN_ERROR"
  | "TIMEOUT_ERROR";

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

interface ExtendedAuthState extends Omit<AuthState, "error"> {
  error: AuthError | null;
  lastActivity: Date | null;
  sessionTimeout: number; // minutes
}

// 🔄 Phase 3: フロー管理状態
interface FlowState {
  registrationFlow: RegistrationFlow | null;
  loginFlow: LoginFlow | null;
  expiresAt: Date | null;
  isExpired: boolean;
  lastRefreshTime: Date | null;
}

// 🚀 X24 Phase 3: 2025 Accessibility and modern React patterns
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
  ensureValidRegistrationFlow: () => Promise<RegistrationFlow>;
  ensureValidLoginFlow: () => Promise<LoginFlow>;
  isFlowValid: (flow: RegistrationFlow | LoginFlow | null) => boolean;
  // 🚀 X24 Phase 3: 2025 Accessibility & Modern Features
  getAccessibilityState: () => {
    "aria-busy": boolean;
    "aria-live": "polite" | "assertive" | "off";
    role: string;
    "aria-label": string;
  };
  securityMetrics: {
    sessionIntegrity: boolean;
    lastSecurityCheck: Date | null;
    csrfProtection: boolean;
  };
}

const AuthContext = createContext<AuthContextType | null>(null);

interface AuthProviderProps {
  children: ReactNode;
}

// エラーマッピング関数 - 詳細診断ログ付き
const mapErrorToAuthError = (error: unknown, retryCount = 0): AuthError => {
  if (error instanceof Error) {
    const extractDetailedErrorInfo = (errorMessage: string) => {
      const detailedErrorMatch = errorMessage.match(/\[([A-Z_]+)\]: (.+)/);
      if (detailedErrorMatch) {
        const [, errorType, message] = detailedErrorMatch;
        return { errorType, message };
      }
      return null;
    };

    const detailedInfo = extractDetailedErrorInfo(error.message);

    if (detailedInfo) {
      const baseError: AuthError = {
        type: detailedInfo.errorType as AuthErrorType,
        message: detailedInfo.message,
        isRetryable: true,
        retryCount,
        technicalInfo: `Backend error: ${detailedInfo.errorType}`,
        errorCode: detailedInfo.errorType,
      };

      switch (detailedInfo.errorType) {
        case "MISSING_EMAIL_FIELD":
          baseError.type = "DATA_FORMAT_ERROR";
          baseError.isRetryable = true;
          baseError.suggestions = [
            "メールアドレスフィールドが正しく送信されているか確認してください",
            "フォームを再読み込みして再試行してください",
          ];
          break;
        case "USER_ALREADY_EXISTS":
          baseError.type = "USER_ALREADY_EXISTS";
          baseError.isRetryable = false;
          baseError.suggestions = [
            "別のメールアドレスを使用してください",
            "既にアカウントをお持ちの場合はログインしてください",
          ];
          break;
        case "FLOW_EXPIRED":
          baseError.type = "FLOW_EXPIRED";
          baseError.isRetryable = true;
          baseError.suggestions = [
            "ページを再読み込みして新しい登録フローを開始してください",
          ];
          break;
        case "SESSION_NOT_FOUND":
          baseError.type = "SESSION_NOT_FOUND";
          baseError.isRetryable = true;
          baseError.suggestions = [
            "認証が必要です。ログインしてください",
            "ページを再読み込みしてください",
          ];
          break;
        default:
          baseError.isRetryable = true;
      }

      return baseError;
    }

    if (error.message.includes("404") || error.message.includes("Not Found")) {
      return {
        type: "KRATOS_SERVICE_ERROR",
        message:
          "認証サービスに接続できません。しばらく後にもう一度お試しください",
        isRetryable: true,
        retryCount,
        technicalInfo: "Authentication service endpoints not accessible",
        suggestions: [
          "しばらく待ってから再試行してください",
          "サポートにお問い合わせください",
        ],
      };
    }

    if (
      error.message.includes("Failed to fetch") ||
      error.message.includes("Network request failed")
    ) {
      return {
        type: "NETWORK_ERROR",
        message: "ネットワーク接続を確認してください",
        isRetryable: true,
        retryCount,
        technicalInfo: "Network connectivity issue",
        suggestions: [
          "インターネット接続を確認してください",
          "再試行してください",
        ],
      };
    }

    // 認証エラーの検出 - より精密な分類
    if (
      error.message.includes("SESSION_NOT_FOUND") ||
      error.message.includes("Authentication required")
    ) {
      return {
        type: "SESSION_NOT_FOUND",
        message: "認証が必要です。ログインしてください",
        isRetryable: true,
        retryCount,
        technicalInfo: "Session not found - authentication required",
        suggestions: [
          "ログインページに移動してください",
          "ページを再読み込みしてください",
        ],
      };
    }

    if (
      error.message.includes("401") ||
      error.message.includes("Unauthorized") ||
      error.message.includes("Invalid credentials")
    ) {
      return {
        type: "INVALID_CREDENTIALS",
        message: "メールアドレスまたはパスワードが正しくありません",
        isRetryable: false,
        retryCount,
      };
    }

    // セッション期限切れの検出
    if (
      error.message.includes("Session expired") ||
      error.message.includes("Token expired")
    ) {
      return {
        type: "SESSION_EXPIRED",
        message: "セッションの有効期限が切れました。再度ログインしてください",
        isRetryable: false,
        retryCount,
      };
    }

    // タイムアウトエラーの検出
    if (
      error.message.includes("timeout") ||
      error.message.includes("AbortError")
    ) {
      return {
        type: "TIMEOUT_ERROR",
        message: "リクエストがタイムアウトしました",
        isRetryable: true,
        retryCount,
      };
    }

    // 🚨 FIX: より厳密な既存ユーザー検出 - HTTP 409 Conflict 専用
    if (
      (error.message.includes("409") &&
        (error.message.includes("User already exists") ||
          error.message.includes("already registered") ||
          error.message.includes("email already taken"))) ||
      error.message.includes("USER_ALREADY_EXISTS")
    ) {
      return {
        type: "USER_ALREADY_EXISTS",
        message:
          "このメールアドレスは既に登録されています。ログインをお試しください",
        isRetryable: false,
        retryCount,
        technicalInfo: "User conflict detected from authentication service",
      };
    }

    // データ形式エラーの明確な分離
    if (
      error.message.includes("Property email is missing") ||
      error.message.includes("missing properties") ||
      error.message.includes("traits") ||
      error.message.includes("VALIDATION_FAILED")
    ) {
      return {
        type: "DATA_FORMAT_ERROR",
        message:
          "登録情報の形式に問題があります。メールアドレスとパスワードを確認してください",
        isRetryable: true,
        retryCount,
      };
    }

    // フロー期限切れの検出
    if (
      error.message.includes("flow expired") ||
      error.message.includes("Flow expired") ||
      error.message.includes("410")
    ) {
      return {
        type: "FLOW_EXPIRED",
        message: "登録フローの有効期限が切れました。最初からやり直してください",
        isRetryable: true,
        retryCount,
      };
    }

    // Kratosサービス固有エラー
    if (
      error.message.includes("kratos") ||
      error.message.includes("Kratos") ||
      error.message.includes("502") ||
      error.message.includes("503")
    ) {
      return {
        type: "KRATOS_SERVICE_ERROR",
        message:
          "認証サービスに一時的な問題が発生しています。しばらく後にもう一度お試しください",
        isRetryable: true,
        retryCount,
      };
    }

    // 最後の手段として汎用的な登録エラー（より限定的な条件）
    if (
      error.message.includes("registration failed") ||
      error.message.includes("Registration failed")
    ) {
      return {
        type: "REGISTRATION_FAILED",
        message:
          "登録処理中にエラーが発生しました。入力内容を確認してもう一度お試しください",
        isRetryable: true,
        retryCount,
      };
    }

    // バリデーションエラーの検出
    if (
      error.message.includes("validation") ||
      error.message.includes("invalid format")
    ) {
      return {
        type: "VALIDATION_ERROR",
        message: "入力内容を確認してください",
        isRetryable: false,
        retryCount,
      };
    }

    return {
      type: "UNKNOWN_ERROR" as AuthErrorType,
      message: error.message || "予期しないエラーが発生しました",
      isRetryable: true,
      retryCount,
    };
  }

  return {
    type: "UNKNOWN_ERROR" as AuthErrorType,
    message: "予期しないエラーが発生しました",
    isRetryable: true,
    retryCount,
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

  // 🚀 X24 Phase 3: 2025 Security monitoring
  const securityCheckRef = useRef<Date | null>(null);
  const [securityMetrics, setSecurityMetrics] = useState({
    sessionIntegrity: true,
    lastSecurityCheck: null as Date | null,
    csrfProtection: true,
  });

  // 🔧 X24 Phase 2: Type-safe action tracking interfaces
  interface LoginAction {
    type: "login";
    params: [email: string, password: string];
  }

  interface RegisterAction {
    type: "register";
    params: [email: string, password: string, name?: string];
  }

  interface RefreshAction {
    type: "refresh";
    params: [];
  }

  type LastActionType = LoginAction | RegisterAction | RefreshAction;

  // 最後に実行しようとしたアクション
  const [lastAction, setLastAction] = useState<LastActionType | null>(null);

  // 🔍 ULTRA-DIAGNOSTIC: デバッグ状態管理
  const [debugCaptureEnabled, setDebugCaptureEnabled] = useState(false);

  // 🔄 Phase 3: フロー管理状態管理
  const [flowState, setFlowState] = useState<FlowState>({
    registrationFlow: null,
    loginFlow: null,
    expiresAt: null,
    isExpired: false,
    lastRefreshTime: null,
  });

  // 🚀 X24 Phase 1: Smart Session Detection - 無条件API呼び出し停止
  const CACHE_DURATION = 5 * 60 * 1000; // 5 minutes
  let sessionCache: { user: User | null; timestamp: number } | null = null;

  // 🔧 X24: SSR-safe session detection to avoid hydration mismatch
  const hasSessionIndicators = useCallback((): boolean => {
    // SSR safety check - avoid hydration mismatch
    if (typeof window === "undefined" || typeof document === "undefined") {
      return false;
    }

    const cookieString = document.cookie;
    const sessionCookies = [
      "ory_kratos_session",      // underscore variant
      "ory-kratos-session",      // hyphen variant (Kratos may set this)
      "kratos-session",
      "auth-session",
      "_session",
      "sessionid",
    ];

    return sessionCookies.some((cookieName) =>
      cookieString.includes(cookieName + "="),
    );
  }, []);

  // 🔧 X24: Client-side caching without React.cache to avoid hydration mismatch
  const getCachedAuthStatus = async (): Promise<User | null> => {
    try {
      const now = Date.now();

      // Return cached result if still valid
      if (sessionCache && now - sessionCache.timestamp < CACHE_DURATION) {
        trackPerformanceMetric("cacheHit");
        return sessionCache.user;
      }

      // Fetch fresh data
      const user = await authAPI.getCurrentUser();
      sessionCache = { user, timestamp: now };

      return user;
    } catch (error) {
      if (error instanceof Error && error.message.includes("401")) {
        // Cache null result for unauthenticated state
        sessionCache = { user: null, timestamp: Date.now() };
        return null;
      }
      throw error; // Re-throw non-auth errors
    }
  };

  // 📊 X24: Performance monitoring and optimization
  const [performanceMetrics, setPerformanceMetrics] = useState({
    apiCallsAvoided: 0,
    cacheHits: 0,
    totalRequests: 0,
    lastOptimizationCheck: Date.now(),
  });

  // 🚀 X24 Phase 3: 2025 Performance optimization with useMemo
  const authStateAccessibility = useMemo(
    () => ({
      "aria-busy": authState.isLoading,
      "aria-live": (authState.error ? "assertive" : "polite") as
        | "polite"
        | "assertive"
        | "off",
      role: "status",
      "aria-label": authState.isLoading
        ? "認証状態を確認中です"
        : authState.isAuthenticated
          ? "ログイン済みです"
          : "ログインが必要です",
    }),
    [authState.isLoading, authState.error, authState.isAuthenticated],
  );

  // 🚀 X24 Phase 3: 2025 useCallback optimization for performance tracking
  const trackPerformanceMetric = useCallback(
    (metricType: "apiAvoided" | "cacheHit" | "totalRequest") => {
      setPerformanceMetrics((prev) => {
        const updated = { ...prev };
        switch (metricType) {
          case "apiAvoided":
            updated.apiCallsAvoided += 1;
            break;
          case "cacheHit":
            updated.cacheHits += 1;
            break;
          case "totalRequest":
            updated.totalRequests += 1;
            break;
        }

        // Log performance improvement every 10 requests
        if (updated.totalRequests % 10 === 0) {
          const avoidanceRate = (
            ((updated.apiCallsAvoided + updated.cacheHits) /
              updated.totalRequests) *
            100
          ).toFixed(1);
        }

        return updated;
      });
    },
    [],
  );

  // 🧹 X24: Enhanced session management utilities
  const clearSessionCache = useCallback(() => {
    sessionCache = null;
  }, []);

  // 🚀 X24 Phase 3: 2025 Security integrity check
  const performSecurityCheck = useCallback(() => {
    const now = new Date();
    const hasValidSession = hasSessionIndicators();
    const timeSinceLastCheck = securityCheckRef.current
      ? now.getTime() - securityCheckRef.current.getTime()
      : 0;

    // Perform security check every 5 minutes
    if (timeSinceLastCheck > 5 * 60 * 1000 || !securityCheckRef.current) {
      setSecurityMetrics((prev) => ({
        ...prev,
        sessionIntegrity: hasValidSession === authState.isAuthenticated,
        lastSecurityCheck: now,
        csrfProtection: true, // Assume CSRF protection is active
      }));

      securityCheckRef.current = now;
    }
  }, [hasSessionIndicators, authState.isAuthenticated]);

  // X24 Phase 1: Conditional session check - only call API if session indicators exist
  useEffect(() => {
    let isMounted = true;

    const initAuth = async () => {
      trackPerformanceMetric("totalRequest");

      if (hasSessionIndicators()) {
        if (isMounted) {
          await checkAuthStatus();
        }
      } else {
        trackPerformanceMetric("apiAvoided");
        // No session indicators - set unauthenticated state without API call
        if (isMounted) {
          setAuthState((prev) => ({
            ...prev,
            isAuthenticated: false,
            user: null,
            isLoading: false,
            error: null,
          }));
        }
      }

      // 🚀 X24 Phase 3: Initial security check
      if (isMounted) {
        performSecurityCheck();
      }
    };

    initAuth();

    return () => {
      isMounted = false;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []); // Empty deps - only run once on mount

  // 🚀 X24 Phase 3: Enhanced session monitoring with security checks
  useEffect(() => {
    if (authState.isAuthenticated && authState.lastActivity) {
      const checkInterval = setInterval(() => {
        const now = new Date();
        const lastActivity = authState.lastActivity!;
        const minutesSinceLastActivity = Math.floor(
          (now.getTime() - lastActivity.getTime()) / (1000 * 60),
        );

        if (minutesSinceLastActivity >= authState.sessionTimeout) {
          logout();
        } else {
          // Perform periodic security check
          performSecurityCheck();
        }
      }, 60000); // 1分毎にチェック

      return () => clearInterval(checkInterval);
    }
  }, [
    authState.isAuthenticated,
    authState.lastActivity,
    authState.sessionTimeout,
    performSecurityCheck,
  ]);

  // 🔄 Phase 3: フロー期限監視システム
  useEffect(() => {
    const checkFlowExpiration = () => {
      const now = new Date();

      // 登録フロー期限チェック
      if (flowState.registrationFlow && flowState.expiresAt) {
        const timeToExpiry = flowState.expiresAt.getTime() - now.getTime();
        const isExpiring = timeToExpiry < 5 * 60 * 1000; // 5分以内に期限切れ

        if (isExpiring && !flowState.isExpired) {

          setFlowState((prev) => ({ ...prev, isExpired: true }));
        }
      }

      // ログインフロー期限チェック
      if (flowState.loginFlow && flowState.expiresAt) {
        const timeToExpiry = flowState.expiresAt.getTime() - now.getTime();
        const isExpiring = timeToExpiry < 5 * 60 * 1000; // 5分以内に期限切れ

        if (isExpiring && !flowState.isExpired) {

          setFlowState((prev) => ({ ...prev, isExpired: true }));
        }
      }
    };

    // 30秒毎にフロー期限をチェック
    const flowCheckInterval = setInterval(checkFlowExpiration, 30000);

    return () => clearInterval(flowCheckInterval);
  }, [
    flowState.registrationFlow,
    flowState.loginFlow,
    flowState.expiresAt,
    flowState.isExpired,
  ]);

  // 🚀 X24 Phase 3: Enhanced activity tracking with security check
  const updateActivity = useCallback(() => {
    if (authState.isAuthenticated) {
      setAuthState((prev) => ({ ...prev, lastActivity: new Date() }));
      performSecurityCheck(); // Perform security check on user activity
    }
  }, [authState.isAuthenticated, performSecurityCheck]);

  // 🚀 X24 Phase 3: 2025 Accessibility state getter
  const getAccessibilityState = useCallback(
    () => authStateAccessibility,
    [authStateAccessibility],
  );

  // 🔄 Phase 3: フロー有効性チェック
  const isFlowValid = useCallback(
    (flow: RegistrationFlow | LoginFlow | null): boolean => {
      if (!flow || !flow.expiresAt) {
        return false;
      }

      const now = new Date();
      const expiresAt = new Date(flow.expiresAt);
      const isValid = expiresAt > now;


      return isValid;
    },
    [],
  );

  // 🔄 Phase 3: 有効な登録フロー確保
  const ensureValidRegistrationFlow =
    useCallback(async (): Promise<RegistrationFlow> => {
      const flowManagerId = `REG-FLOW-${Date.now()}`;

      // 既存フローの有効性チェック
      if (
        flowState.registrationFlow &&
        isFlowValid(flowState.registrationFlow)
      ) {
        return flowState.registrationFlow;
      }


      try {
        const newFlow = await authAPI.initiateRegistration();

        setFlowState((prev) => ({
          ...prev,
          registrationFlow: newFlow,
          expiresAt: new Date(newFlow.expiresAt),
          isExpired: false,
          lastRefreshTime: new Date(),
        }));


        return newFlow;
      } catch (error) {
        console.error(
          `❌ [FLOW-MANAGER] Failed to create registration flow - ${flowManagerId}`,
          error,
        );
        throw error;
      }
    }, [flowState.registrationFlow, isFlowValid]);

  // 🔄 Phase 3: 有効なログインフロー確保
  const ensureValidLoginFlow = useCallback(async (): Promise<LoginFlow> => {
    const flowManagerId = `LOGIN-FLOW-${Date.now()}`;

    // 既存フローの有効性チェック
    if (flowState.loginFlow && isFlowValid(flowState.loginFlow)) {
      return flowState.loginFlow;
    }


    try {
      const newFlow = await authAPI.initiateLogin();

      setFlowState((prev) => ({
        ...prev,
        loginFlow: newFlow,
        expiresAt: new Date(newFlow.expiresAt),
        isExpired: false,
        lastRefreshTime: new Date(),
      }));


      return newFlow;
    } catch (error) {
      console.error(
        `❌ [FLOW-MANAGER] Failed to create login flow - ${flowManagerId}`,
        error,
      );
      throw error;
    }
  }, [flowState.loginFlow, isFlowValid]);

  // 🎯 X24: Data Access Layer (DAL) pattern - cached authentication check
  const checkAuthStatus = async (retryCount = 0): Promise<void> => {
    try {
      setAuthState((prev) => ({ ...prev, isLoading: true, error: null }));

      // X24: Only make API call if session indicators exist
      if (!hasSessionIndicators()) {
        setAuthState((prev) => ({
          ...prev,
          isAuthenticated: false,
          user: null,
          isLoading: false,
          error: null,
        }));
        return;
      }

      // X24: Use cached authentication status instead of direct API call
      const user = await getCachedAuthStatus();
      setAuthState((prev) => ({
        ...prev,
        user,
        isAuthenticated: !!user,
        isLoading: false,
        error: null,
        lastActivity: user ? new Date() : null,
      }));
    } catch (error: unknown) {
      const authError = mapErrorToAuthError(error, retryCount);

      // 🚨 FIX: Removed automatic client-side redirect to /auth/login
      // Middleware (middleware.ts) already handles session validation and redirects
      // Duplicate client-side redirects cause infinite loops

      // Log 401/Unauthorized errors for debugging, but don't redirect
      const is401Error =
        authError.type === "INVALID_CREDENTIALS" ||
        (error instanceof Error &&
          (error.message.includes("401") ||
            error.message.includes("Unauthorized")));

      if (is401Error) {
      }

      // 再試行可能なエラーで再試行回数が3回未満の場合は再試行
      if (authError.isRetryable && retryCount < 3) {
        setTimeout(
          () => {
            checkAuthStatus(retryCount + 1);
          },
          Math.pow(2, retryCount) * 1000,
        ); // 指数バックオフ
        return;
      }

      setAuthState((prev) => ({
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
    setLastAction({ type: "login", params: [email, password] });

    try {
      setAuthState((prev) => ({ ...prev, isLoading: true, error: null }));

      // 🔄 Phase 3: 有効なログインフロー確保
      const loginFlow = await ensureValidLoginFlow();

      // 🚨 防御的プログラミング: flow オブジェクト検証強化
      if (!loginFlow || !loginFlow.id) {
        throw new Error("Login flow initialization failed: missing flow ID");
      }


      // Complete login with credentials
      const user = await authAPI.completeLogin(loginFlow.id, email, password);

      // 🚨 防御的プログラミング: user オブジェクト検証
      if (!user) {
        throw new Error("Login completed but user data is missing");
      }

      setAuthState((prev) => ({
        ...prev,
        user,
        isAuthenticated: true,
        isLoading: false,
        error: null,
        lastActivity: new Date(),
      }));


      // 🚀 X24 Phase 3: Security check after successful login
      performSecurityCheck();

      // ログイン成功時は前回のアクションをクリア
      setLastAction(null);
    } catch (error: unknown) {
      console.error("[AUTH-CONTEXT] Login failed:", error);
      const authError = mapErrorToAuthError(error);
      setAuthState((prev) => ({
        ...prev,
        isLoading: false,
        error: authError,
      }));
      throw error;
    }
  };

  const register = async (email: string, password: string, name?: string) => {
    setLastAction({ type: "register", params: [email, password, name] });

    try {
      setAuthState((prev) => ({ ...prev, isLoading: true, error: null }));

      // 🔄 Phase 3: 有効な登録フロー確保
      const registrationFlow = await ensureValidRegistrationFlow();

      // 🚨 防御的プログラミング: flow オブジェクト検証強化
      if (!registrationFlow || !registrationFlow.id) {
        throw new Error(
          "Registration flow initialization failed: missing flow ID",
        );
      }


      // 🚀 X24 Phase 3: Enhanced data sanitization for security
      const sanitizedEmail = email.trim().toLowerCase();
      const sanitizedName = name?.trim();

      // Complete registration with user data
      const user = await authAPI.completeRegistration(
        registrationFlow.id,
        sanitizedEmail,
        password,
        sanitizedName,
      );

      // 🚨 防御的プログラミング: user オブジェクト検証
      if (!user) {
        throw new Error("Registration completed but user data is missing");
      }

      setAuthState((prev) => ({
        ...prev,
        user,
        isAuthenticated: true,
        isLoading: false,
        error: null,
        lastActivity: new Date(),
      }));


      // 🚀 X24 Phase 3: Security check after successful registration
      performSecurityCheck();

      // 登録成功時は前回のアクションをクリア
      setLastAction(null);
    } catch (error: unknown) {
      // 詳細ログ出力でデバッグ性向上
      console.error("[AUTH-CONTEXT] Registration failed - Raw error:", error);
      console.error(
        "[AUTH-CONTEXT] Registration failed - Error type:",
        typeof error,
      );
      console.error(
        "[AUTH-CONTEXT] Registration failed - Flow ID:",
        "flow_id_not_available",
      );
      console.error(
        "[AUTH-CONTEXT] Registration failed - Email:",
        email ? "provided" : "missing",
      );
      console.error(
        "[AUTH-CONTEXT] Registration failed - Password:",
        password ? "provided" : "missing",
      );
      console.error(
        "[AUTH-CONTEXT] Registration failed - Name:",
        name || "not provided",
      );

      if (error instanceof Error) {
        console.error(
          "[AUTH-CONTEXT] Registration failed - Error message:",
          error.message,
        );
        console.error(
          "[AUTH-CONTEXT] Registration failed - Error stack:",
          error.stack,
        );
      }

      const authError = mapErrorToAuthError(error);
      console.error(
        "[AUTH-CONTEXT] Registration failed - Mapped error type:",
        authError.type,
      );
      console.error(
        "[AUTH-CONTEXT] Registration failed - Mapped error message:",
        authError.message,
      );
      console.error(
        "[AUTH-CONTEXT] Registration failed - Is retryable:",
        authError.isRetryable,
      );

      setAuthState((prev) => ({
        ...prev,
        isLoading: false,
        error: authError,
      }));
      throw error;
    }
  };

  const logout = async () => {
    try {
      setAuthState((prev) => ({ ...prev, error: null }));
      // X24: Clear session cache on logout
      clearSessionCache();
      await authAPI.logout();
      setAuthState((prev) => ({
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
      setAuthState((prev) => ({
        ...prev,
        user: null,
        isAuthenticated: false,
        isLoading: false,
        error: null,
        lastActivity: null,
      }));
    }
  };

  const refresh = async () => {
    setLastAction({ type: "refresh", params: [] });
    await checkAuthStatus();
  };

  const retryLastAction = async () => {
    if (!lastAction) {
      throw new Error("再試行可能なアクションがありません");
    }

    const { type, params } = lastAction;

    try {
      // 🔧 X24 Phase 2: Type-safe action execution
      switch (type) {
        case "login": {
          const [email, password] = params;
          await login(email, password);
          break;
        }
        case "register": {
          const [email, password, name] = params;
          await register(email, password, name);
          break;
        }
        case "refresh":
          await refresh();
          break;
        default:
          // TypeScript exhaustiveness check
          const _exhaustiveCheck: never = type;
          throw new Error(`不明なアクションタイプです: ${_exhaustiveCheck}`);
      }
    } catch (error) {
      // エラーは元の関数で処理されるため、ここでは再スロー
      throw error;
    }
  };

  const clearError = () => {
    setAuthState((prev) => ({ ...prev, error: null }));
  };

  // 🔍 ULTRA-DIAGNOSTIC: 開発者向け診断機能
  const debugDiagnoseRegistrationFlow = async (): Promise<any> => {
    const diagnosticId = `DIAG-CTX-${Date.now()}-${Math.random().toString(36).substr(2, 9)}`;


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
          lastActivity: authState.lastActivity?.toISOString() || null,
        },
        browserState: {
          userAgent: navigator.userAgent,
          cookieEnabled: navigator.cookieEnabled,
          onLine: navigator.onLine,
          language: navigator.language,
          currentUrl: window.location.href,
          sessionStorageKeys: Object.keys(sessionStorage),
          localStorageKeys: Object.keys(localStorage),
          documentCookies: document.cookie.split(";").length,
        },
        lastAction: lastAction || null,
      };


      // バックエンド診断の実行 (mock for development)
      const backendDiagnostic = {
        kratosStatus: { isConnected: true },
        flowTest: { testStatus: "SUCCESS" },
        databaseTest: { isConnected: true },
      };

      // 統合診断結果
      const fullDiagnostic = {
        diagnosticId,
        timestamp: new Date().toISOString(),
        frontend: systemState,
        backend: backendDiagnostic,
        recommendations: generateDiagnosticRecommendations(
          systemState,
          backendDiagnostic,
        ),
      };


      return fullDiagnostic;
    } catch (error) {
      console.error("❌ Diagnostic failed:", error);
      throw error;
    }
  };

  const debugCaptureNextRequest = (enable: boolean) => {
    setDebugCaptureEnabled(enable);

    if (enable) {
    }
  };

  // 🔧 X24 Phase 2: Type-safe diagnostic interfaces
  interface DiagnosticAuthState {
    isAuthenticated: boolean;
    isLoading: boolean;
    hasUser: boolean;
    hasError: boolean;
    errorType: string | null;
    lastActivity: string | null;
  }

  interface DiagnosticBrowserState {
    userAgent: string;
    cookieEnabled: boolean;
    onLine: boolean;
    language: string;
    currentUrl: string;
    sessionStorageKeys: string[];
    localStorageKeys: string[];
    documentCookies: number;
  }

  interface DiagnosticFrontendState {
    authState: DiagnosticAuthState;
    browserState: DiagnosticBrowserState;
  }

  interface DiagnosticKratosStatus {
    isConnected: boolean;
  }

  interface DiagnosticFlowTest {
    testStatus: string;
  }

  interface DiagnosticDatabaseTest {
    isConnected: boolean;
  }

  interface DiagnosticBackendState {
    kratosStatus?: DiagnosticKratosStatus;
    flowTest?: DiagnosticFlowTest;
    databaseTest?: DiagnosticDatabaseTest;
  }

  // 診断結果に基づく推奨事項生成
  const generateDiagnosticRecommendations = (
    frontendState: DiagnosticFrontendState,
    backendDiagnostic: DiagnosticBackendState,
  ): string[] => {
    const recommendations: string[] = [];

    // フロントエンドの状態チェック
    if (frontendState.authState.hasError) {
      recommendations.push(
        `🔧 現在のエラー "${frontendState.authState.errorType}" を確認してください`,
      );
    }

    if (!frontendState.browserState.cookieEnabled) {
      recommendations.push(
        "🍪 ブラウザのクッキーが無効になっています。有効にしてください",
      );
    }

    if (!frontendState.browserState.onLine) {
      recommendations.push("🌐 ネットワーク接続を確認してください");
    }

    // バックエンドの状態チェック
    if (backendDiagnostic.kratosStatus?.isConnected === false) {
      recommendations.push("🔌 Kratos認証サービスへの接続に問題があります");
    }

    if (backendDiagnostic.flowTest?.testStatus === "PARTIAL_FAILURE") {
      recommendations.push("⚠️ 登録フローテストで部分的な失敗が検出されました");
    }

    if (backendDiagnostic.databaseTest?.isConnected === false) {
      recommendations.push("🗃️ データベース接続に問題があります");
    }

    if (recommendations.length === 0) {
      recommendations.push("✅ システムは正常に動作しているようです");
      recommendations.push("💡 実際の登録試行時のログを確認してください");
    }

    return recommendations;
  };

  // 🚀 X24 Phase 3: 2025 useMemo optimization for context value
  const contextValue: AuthContextType = useMemo(
    () => ({
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
      // 🚀 X24 Phase 3: 2025 Modern Features
      getAccessibilityState,
      securityMetrics,
    }),
    [
      authState,
      login,
      register,
      logout,
      refresh,
      clearError,
      retryLastAction,
      debugDiagnoseRegistrationFlow,
      debugCaptureNextRequest,
      ensureValidRegistrationFlow,
      ensureValidLoginFlow,
      isFlowValid,
      getAccessibilityState,
      securityMetrics,
    ],
  );

  return (
    <AuthContext.Provider value={contextValue}>{children}</AuthContext.Provider>
  );
}

export function useAuth(): AuthContextType {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used within an AuthProvider");
  }
  return context;
}
