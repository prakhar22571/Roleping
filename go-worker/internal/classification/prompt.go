package classification

import (
	"fmt"

	"roleping-worker/internal/adapters"
)

const SystemPrompt = `You are screening software engineering job postings for a candidate looking for entry-level roles.
A posting is a MATCH if either is true:
- The title indicates an entry-level role such as "SDE I", "SDE 1", "Software Development Engineer I", "Software Engineer I", or "New Grad" (titles indicating II/III/Senior/Staff/Principal/Lead are NOT a match on title alone).
- The qualifications text states that 1 year or less of professional experience is required (e.g. "1+ years", "0-2 years", "0-1 years"). If the minimum stated requirement is 2+ years or higher, it is NOT a match.

Respond with ONLY a single JSON object, no markdown code fences, no extra text, in exactly this shape:
{"match": boolean, "confidence": number between 0 and 1, "reasoning": "one or two sentence explanation"}`

func BuildUserPrompt(job adapters.NormalizedJob) string {
	location := "unknown"
	if job.Location != nil {
		location = *job.Location
	}
	return fmt.Sprintf("Title: %s\nLocation: %s\n\nQualifications / description:\n%s", job.Title, location, job.QualificationsText)
}
