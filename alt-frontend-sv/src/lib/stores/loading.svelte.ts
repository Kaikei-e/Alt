/**
 * Desktop loading state store
 *
 * Used to show/hide the SystemLoader component during CSR data fetching.
 * Pages should set this to true when starting data fetch and false when complete.
 */
class LoadingStore {
	isDesktopLoading = $state(false);

	setLoading(loading: boolean) {
		this.isDesktopLoading = loading;
	}

	startLoading() {
		this.isDesktopLoading = true;
	}

	stopLoading() {
		this.isDesktopLoading = false;
	}
}

export const loadingStore = new LoadingStore();
