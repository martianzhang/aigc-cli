package service

// TaskResume returns the recovery line for an APIMart-style task. The leading
// newline lets it be appended to an existing error without breaking its text.
func TaskResume(id string) string {
	if id == "" {
		return ""
	}
	return "\nResume: aigc-cli task " + id
}

// JobResume returns the recovery line for an OpenRouter video job.
func JobResume(id string) string {
	if id == "" {
		return ""
	}
	return "\nResume: aigc-cli video --job-id " + id
}
