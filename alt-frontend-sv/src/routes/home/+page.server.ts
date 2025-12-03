import type { ServerLoad } from "@sveltejs/kit";
import { getFeedStats, getTodayUnreadCount } from "$lib/api";

export const load: ServerLoad = async ({ request }) => {
	// すべてのcookieを文字列として取得
	const cookieHeader = request.headers.get("cookie") || "";

	// 今日の開始時刻を取得（UTC）
	const now = new Date();
	const startOfDay = new Date(
		Date.UTC(
			now.getUTCFullYear(),
			now.getUTCMonth(),
			now.getUTCDate(),
			0,
			0,
			0,
			0,
		),
	);
	const since = startOfDay.toISOString();

	try {
		// 並列でデータを取得
		const [stats, unreadCount] = await Promise.all([
			getFeedStats(cookieHeader),
			getTodayUnreadCount(cookieHeader, since),
		]);

		return {
			stats,
			unreadCount: unreadCount.count,
		};
	} catch (error) {
		console.error("Failed to load stats:", error);
		return {
			stats: {
				feed_amount: { amount: 0 },
				total_articles: { amount: 0 },
				unsummarized_articles: { amount: 0 },
			},
			unreadCount: 0,
			error: "Failed to load statistics",
		};
	}
};
