package middleware

func (l *UserQuotaLimiter) TrackedUsers(op OperationClass) int {
	return l.ops[op].trackedUsers()
}
