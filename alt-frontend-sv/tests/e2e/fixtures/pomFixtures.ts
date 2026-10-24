/**
 * Custom Playwright fixtures for POM injection.
 *
 * Every spec imports { test, expect } from this file instead of
 * @playwright/test, which is also how the hygiene auto-fixtures in `baseTest`
 * (third-party request blocking, uncaught-exception detection) reach every
 * test. Each POM is lazily created only when destructured in the test.
 */

// Auth POMs
import { LoginPage } from "../pages/auth/LoginPage";
import { RegisterPage } from "../pages/auth/RegisterPage";
// Component POMs
import { SidebarComponent } from "../pages/components/SidebarComponent";
import { DesktopAcolyteNewPage } from "../pages/desktop/DesktopAcolyteNewPage";
import { DesktopAcolyteReportPage } from "../pages/desktop/DesktopAcolyteReportPage";
import { DesktopAugurPage } from "../pages/desktop/DesktopAugurPage";
import { DesktopEveningPulsePage } from "../pages/desktop/DesktopEveningPulsePage";
import { DesktopFavoritesPage } from "../pages/desktop/DesktopFavoritesPage";
// Desktop POMs
import { DesktopFeedsPage } from "../pages/desktop/DesktopFeedsPage";
import { DesktopJobStatusPage } from "../pages/desktop/DesktopJobStatusPage";
import { DesktopKnowledgeHomeAdminPage } from "../pages/desktop/DesktopKnowledgeHomeAdminPage";
import { DesktopKnowledgeHomePage } from "../pages/desktop/DesktopKnowledgeHomePage";
import { DesktopMorningLetterPage } from "../pages/desktop/DesktopMorningLetterPage";
import { DesktopRecapCardsPage } from "../pages/desktop/DesktopRecapCardsPage";
import { DesktopRecapPage } from "../pages/desktop/DesktopRecapPage";
import { DesktopSearchPage } from "../pages/desktop/DesktopSearchPage";
import { DesktopSettingsPage } from "../pages/desktop/DesktopSettingsPage";
import { DesktopStatsPage } from "../pages/desktop/DesktopStatsPage";
import { DesktopSystemLoaderPage } from "../pages/desktop/DesktopSystemLoaderPage";
import { DesktopTagArticlesPage } from "../pages/desktop/DesktopTagArticlesPage";
import { DesktopTagTrailPage } from "../pages/desktop/DesktopTagTrailPage";
import { DesktopViewedPage } from "../pages/desktop/DesktopViewedPage";
import { Mobile3DayRecapPage } from "../pages/mobile/Mobile3DayRecapPage";
import { MobileEveningPulsePage } from "../pages/mobile/MobileEveningPulsePage";
import { MobileFavoritesPage } from "../pages/mobile/MobileFavoritesPage";
// Mobile POMs
import { MobileFeedsPage } from "../pages/mobile/MobileFeedsPage";
import { MobileJobStatusPage } from "../pages/mobile/MobileJobStatusPage";
import { MobileKnowledgeHomePage } from "../pages/mobile/MobileKnowledgeHomePage";
import { MobileManagePage } from "../pages/mobile/MobileManagePage";
import { MobileMorningLetterPage } from "../pages/mobile/MobileMorningLetterPage";
import { MobileRecapCardsPage } from "../pages/mobile/MobileRecapCardsPage";
import { MobileRecapPage } from "../pages/mobile/MobileRecapPage";
import { MobileSearchPage } from "../pages/mobile/MobileSearchPage";
import { MobileStatsPage } from "../pages/mobile/MobileStatsPage";
import { MobileSwipePage } from "../pages/mobile/MobileSwipePage";
import { MobileTagArticlesPage } from "../pages/mobile/MobileTagArticlesPage";
import { MobileTagTrailPage } from "../pages/mobile/MobileTagTrailPage";
import { MobileViewedPage } from "../pages/mobile/MobileViewedPage";
import { test as base } from "./baseTest";

type PomFixtures = {
	// Auth
	loginPage: LoginPage;
	registerPage: RegisterPage;
	// Desktop
	desktopAcolyteNewPage: DesktopAcolyteNewPage;
	desktopAcolyteReportPage: DesktopAcolyteReportPage;
	desktopFeedsPage: DesktopFeedsPage;
	desktopKnowledgeHomeAdminPage: DesktopKnowledgeHomeAdminPage;
	desktopKnowledgeHomePage: DesktopKnowledgeHomePage;
	desktopRecapPage: DesktopRecapPage;
	desktopRecapCardsPage: DesktopRecapCardsPage;
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
	mobileKnowledgeHomePage: MobileKnowledgeHomePage;
	mobileSwipePage: MobileSwipePage;
	mobileRecapPage: MobileRecapPage;
	mobileRecapCardsPage: MobileRecapCardsPage;
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
	desktopAcolyteNewPage: async ({ page }, use) => {
		await use(new DesktopAcolyteNewPage(page));
	},
	desktopAcolyteReportPage: async ({ page }, use) => {
		await use(new DesktopAcolyteReportPage(page));
	},
	desktopFeedsPage: async ({ page }, use) => {
		await use(new DesktopFeedsPage(page));
	},
	desktopKnowledgeHomeAdminPage: async ({ page }, use) => {
		await use(new DesktopKnowledgeHomeAdminPage(page));
	},
	desktopKnowledgeHomePage: async ({ page }, use) => {
		await use(new DesktopKnowledgeHomePage(page));
	},
	desktopRecapPage: async ({ page }, use) => {
		await use(new DesktopRecapPage(page));
	},
	desktopRecapCardsPage: async ({ page }, use) => {
		await use(new DesktopRecapCardsPage(page));
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
	mobileKnowledgeHomePage: async ({ page }, use) => {
		await use(new MobileKnowledgeHomePage(page));
	},
	mobileSwipePage: async ({ page }, use) => {
		await use(new MobileSwipePage(page));
	},
	mobileRecapPage: async ({ page }, use) => {
		await use(new MobileRecapPage(page));
	},
	mobileRecapCardsPage: async ({ page }, use) => {
		await use(new MobileRecapCardsPage(page));
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
});

export { expect } from "./baseTest";
