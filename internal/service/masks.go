package service

// MaskKey returns a masked form of an API key (last 4 chars), or "***" if too short.
func MaskKey(key string) string {
	if len(key) > 4 {
		return "..." + key[len(key)-4:]
	}
	return "***"
}
