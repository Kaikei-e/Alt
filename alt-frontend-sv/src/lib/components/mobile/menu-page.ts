import {
	getVisibleMobileMenuSections,
	MOBILE_MENU_SECTIONS,
	type MobileMenuItem,
	type MobileMenuSection,
} from "$lib/config/navigation";

export const MENU_SECTIONS = MOBILE_MENU_SECTIONS;
export const getVisibleSections = getVisibleMobileMenuSections;

export type MenuGridItem = MobileMenuItem;
export type MenuSection = MobileMenuSection;
