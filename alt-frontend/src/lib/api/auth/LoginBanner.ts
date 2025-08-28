export interface LoginBannerConfig {
  autoRemoveDelay?: number; // milliseconds
  loginUrl?: string;
}

const defaultConfig: Required<LoginBannerConfig> = {
  autoRemoveDelay: 30000, // 30 seconds
  loginUrl: "/auth/login"
};

export class LoginBanner {
  private config: Required<LoginBannerConfig>;

  constructor(config: LoginBannerConfig = {}) {
    this.config = { ...defaultConfig, ...config };
  }

  show(): void {
    // Create and show a non-intrusive login banner instead of redirecting
    const existingBanner = document.querySelector('#auth-required-banner');
    if (existingBanner) {
      return; // Banner already shown
    }

    const banner = document.createElement('div');
    banner.id = 'auth-required-banner';
    banner.style.cssText = `
      position: fixed;
      top: 0;
      left: 0;
      right: 0;
      background: linear-gradient(135deg, #ff6b6b, #ee5a24);
      color: white;
      padding: 12px 16px;
      text-align: center;
      font-family: system-ui, -apple-system, sans-serif;
      font-size: 14px;
      z-index: 10000;
      box-shadow: 0 2px 8px rgba(0,0,0,0.2);
      transform: translateY(-100%);
      transition: transform 0.3s ease-out;
    `;

    banner.innerHTML = `
      <div style="display: flex; align-items: center; justify-content: center; gap: 12px;">
        <span>🔒 セッションが切れています</span>
        <button onclick="window.location.href='${this.config.loginUrl}?return_to=' + encodeURIComponent(window.location.href)" 
                style="background: rgba(255,255,255,0.2); border: 1px solid rgba(255,255,255,0.3); color: white; padding: 4px 12px; border-radius: 4px; cursor: pointer; font-size: 12px;">
          再ログイン
        </button>
        <button onclick="this.parentElement.parentElement.remove()" 
                style="background: transparent; border: none; color: white; cursor: pointer; font-size: 16px; padding: 0 4px;">
          ×
        </button>
      </div>
    `;

    document.body.prepend(banner);
    
    // Animate in
    requestAnimationFrame(() => {
      banner.style.transform = 'translateY(0)';
    });

    // Auto-remove after configured delay
    setTimeout(() => {
      if (banner.parentNode) {
        banner.style.transform = 'translateY(-100%)';
        setTimeout(() => banner.remove(), 300);
      }
    }, this.config.autoRemoveDelay);
  }

  static hide(): void {
    const banner = document.querySelector('#auth-required-banner');
    if (banner) {
      (banner as HTMLElement).style.transform = 'translateY(-100%)';
      setTimeout(() => banner.remove(), 300);
    }
  }
}