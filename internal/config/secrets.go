package config

func MaskedSecret(value string) string {
	if value == "" {
		return ""
	}
	return "********"
}
