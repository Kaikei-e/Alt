/**
 * AdminMonitorService client for Connect-RPC.
 *
 * Routes through the SvelteKit proxy at /api/v2/alt.admin_monitor.v1.AdminMonitorService
 * which forwards to the BFF, which in turn hits alt-backend with a service token.
 */

import { createClient } from "@connectrpc/connect";
import type { Client, Transport } from "@connectrpc/connect";

import { AdminMonitorService } from "$lib/gen/alt/admin_monitor/v1/admin_monitor_pb";

export type AdminMonitorClient = Client<typeof AdminMonitorService>;

export function createAdminMonitorClient(
	transport: Transport,
): AdminMonitorClient {
	return createClient(AdminMonitorService, transport);
}
