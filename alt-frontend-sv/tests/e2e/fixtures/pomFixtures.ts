/**
 * Custom Playwright fixtures for POM injection.
 * Specs import { test, expect } from this file instead of @playwright/test.
 * Each POM is lazily created only when destructured in the test.
 */
import { test as base } from "@playwright/test";

// Auth POMs
import { LoginPage } from "../pages/auth/LoginPage";
import { RegisterPage } from "../pages/auth/RegisterPage";

// Desktop POMs
import { DesktopFeedsPage } from "../pages/desktop/DesktopFeedsPage";
import { DesktopRecapPage } from "../pages/desktop/DesktopRecapPage";
import { DesktopSearchPage } from "../pages/desktop/DesktopSearchPage";
import { DesktopSettingsPage } from "../pages/desktop/DesktopSettingsPage";
import { DesktopAugurPage } from "../pages/desktop/DesktopAugurPage";
import { DesktopMorningLetterPage } from "../pages/desktop/DesktopMorningLetterPage";
import { DesktopSystemLoaderPage } from "../pages/desktop/DesktopSystemLoaderPage";
import { DesktopTagTrailPage } from "../pages/desktop/DesktopTagTrailPage";
import { DesktopFavoritesPage } from "../pages/desktop/DesktopFavoritesPage";
import { DesktopViewedPage } from "../pages/desktop/DesktopViewedPage";
import { DesktopStatsPage } from "../pages/desktop/DesktopStatsPage";
import { DesktopEveningPulsePage } from "../pages/desktop/DesktopEveningPulsePage";
import { DesktopJobStatusPage } from "../pages/desktop/DesktopJobStatusPage";
import { DesktopTagArticlesPage } from "../pages/desktop/DesktopTagArticlesPage";

// Mobile POMs
import { MobileFeedsPage } from "../pages/mobile/MobileFeedsPage";
import { MobileSwipePage } from "../pages/mobile/MobileSwipePage";
import { MobileRecapPage } from "../pages/mobile/MobileRecapPage";
import { MobileStatsPage } from "../pages/mobile/MobileStatsPage";
import { MobileSearchPage } from "../pages/mobile/MobileSearchPage";
import { MobileManagePage } from "../pages/mobile/MobileManagePage";
import { MobileViewedPage } from "../pages/mobile/MobileViewedPage";
import { MobileTagTrailPage } from "../pages/mobile/MobileTagTrailPage";
import { MobileMorningLetterPage } from "../pages/mobile/MobileMorningLetterPage";
import { MobileEveningPulsePage } from "../pages/mobile/MobileEveningPulsePage";
import { Mobile3DayRecapPage } from "../pages/mobile/Mobile3DayRecapPage";
import { MobileJobStatusPage } from "../pages/mobile/MobileJobStatusPage";
import { MobileTagArticlesPage } from "../pages/mobile/MobileTagArticlesPage";
import { MobileFavoritesPage } from "../pages/mobile/MobileFavoritesPage";

// Component POMs
import { SidebarComponent } from "../pages/components/SidebarComponent";
import { FloatingMenuComponent } from "../pages/components/FloatingMenuComponent";

type PomFixtures = {
	// Auth
	loginPage: LoginPage;
	registerPage: RegisterPage;
	// Desktop
	desktopFeedsPage: DesktopFeedsPage;
	desktopRecapPage: DesktopRecapPage;
	desktopSearchPage: DesktopSearchPage;
	desktopSettingsPage: DesktopSettingsPage;
	desktopAugurPage: DesktopAugurPage;
	desktopMorningLetterPage: DesktopMorningLetterPage;
	desktopSystemLoaderPage: DesktopSystemLoaderPage;
	desktopTagTrailPage: DesktopTagTrailPage;
	desktopFavoritesPage: DesktopFavoritesPage;
	desktopViewedPage: DesktopViewedPage;
	desktopStatsPage: DesktopStatsPage;
	desktopEveningPulsePage: DesktopEveningPulsePage;
	desktopJobStatusPage: DesktopJobStatusPage;
	desktopTagArticlesPage: DesktopTagArticlesPage;
	// Mobile
	mobileFeedsPage: MobileFeedsPage;
	mobileSwipePage: MobileSwipePage;
	mobileRecapPage: MobileRecapPage;
	mobileStatsPage: MobileStatsPage;
	mobileSearchPage: MobileSearchPage;
	mobileManagePage: MobileManagePage;
	mobileViewedPage: MobileViewedPage;
	mobileTagTrailPage: MobileTagTrailPage;
	mobileMorningLetterPage: MobileMorningLetterPage;
	mobileEveningPulsePage: MobileEveningPulsePage;
	mobile3DayRecapPage: Mobile3DayRecapPage;
	mobileJobStatusPage: MobileJobStatusPage;
	mobileTagArticlesPage: MobileTagArticlesPage;
	mobileFavoritesPage: MobileFavoritesPage;
	// Components
	sidebar: SidebarComponent;
	floatingMenu: FloatingMenuComponent;
};

export const test = base.extend<PomFixtures>({
	// Auth
	loginPage: async ({ page }, use) => {
		await use(new LoginPage(page));
	},
	registerPage: async ({ page }, use) => {
		await use(new RegisterPage(page));
	},
	// Desktop
	desktopFeedsPage: async ({ page }, use) => {
		await use(new DesktopFeedsPage(page));
	},
	desktopRecapPage: async ({ page }, use) => {
		await use(new DesktopRecapPage(page));
	},
	desktopSearchPage: async ({ page }, use) => {
		await use(new DesktopSearchPage(page));
	},
	desktopSettingsPage: async ({ page }, use) => {
		await use(new DesktopSettingsPage(page));
	},
	desktopAugurPage: async ({ page }, use) => {
		await use(new DesktopAugurPage(page));
	},
	desktopMorningLetterPage: async ({ page }, use) => {
		await use(new DesktopMorningLetterPage(page));
	},
	desktopSystemLoaderPage: async ({ page }, use) => {
		await use(new DesktopSystemLoaderPage(page));
	},
	desktopTagTrailPage: async ({ page }, use) => {
		await use(new DesktopTagTrailPage(page));
	},
	desktopFavoritesPage: async ({ page }, use) => {
		await use(new DesktopFavoritesPage(page));
	},
	desktopViewedPage: async ({ page }, use) => {
		await use(new DesktopViewedPage(page));
	},
	desktopStatsPage: async ({ page }, use) => {
		await use(new DesktopStatsPage(page));
	},
	desktopEveningPulsePage: async ({ page }, use) => {
		await use(new DesktopEveningPulsePage(page));
	},
	desktopJobStatusPage: async ({ page }, use) => {
		await use(new DesktopJobStatusPage(page));
	},
	desktopTagArticlesPage: async ({ page }, use) => {
		await use(new DesktopTagArticlesPage(page));
	},
	// Mobile
	mobileFeedsPage: async ({ page }, use) => {
		await use(new MobileFeedsPage(page));
	},
	mobileSwipePage: async ({ page }, use) => {
		await use(new MobileSwipePage(page));
	},
	mobileRecapPage: async ({ page }, use) => {
		await use(new MobileRecapPage(page));
	},
	mobileStatsPage: async ({ page }, use) => {
		await use(new MobileStatsPage(page));
	},
	mobileSearchPage: async ({ page }, use) => {
		await use(new MobileSearchPage(page));
	},
	mobileManagePage: async ({ page }, use) => {
		await use(new MobileManagePage(page));
	},
	mobileViewedPage: async ({ page }, use) => {
		await use(new MobileViewedPage(page));
	},
	mobileTagTrailPage: async ({ page }, use) => {
		await use(new MobileTagTrailPage(page));
	},
	mobileMorningLetterPage: async ({ page }, use) => {
		await use(new MobileMorningLetterPage(page));
	},
	mobileEveningPulsePage: async ({ page }, use) => {
		await use(new MobileEveningPulsePage(page));
	},
	mobile3DayRecapPage: async ({ page }, use) => {
		await use(new Mobile3DayRecapPage(page));
	},
	mobileJobStatusPage: async ({ page }, use) => {
		await use(new MobileJobStatusPage(page));
	},
	mobileTagArticlesPage: async ({ page }, use) => {
		await use(new MobileTagArticlesPage(page));
	},
	mobileFavoritesPage: async ({ page }, use) => {
		await use(new MobileFavoritesPage(page));
	},
	// Components
	sidebar: async ({ page }, use) => {
		await use(new SidebarComponent(page));
	},
	floatingMenu: async ({ page }, use) => {
		await use(new FloatingMenuComponent(page));
	},
});

export { expect } from "@playwright/test";
