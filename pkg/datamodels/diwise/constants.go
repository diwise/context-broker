package diwise

const urnPrefix string = "urn:ngsi-ld:"

const (
	//ExerciseTrailTypeName is a type name constant for ExerciseTrail
	ExerciseTrailTypeName string = "ExerciseTrail"
	//ExerciseTrailIDPrefix contains the mandatory prefix for ExerciseTrail ID:s
	ExerciseTrailIDPrefix string = urnPrefix + ExerciseTrailTypeName + ":"
	//SportsFieldTypeName is a type name constant for SportsField
	SportsFieldTypeName string = "SportsField"
	//SportsFieldIDPrefix contains the mandatory prefix for SportsField ID:s
	SportsFieldIDPrefix string = urnPrefix + SportsFieldTypeName + ":"
)
