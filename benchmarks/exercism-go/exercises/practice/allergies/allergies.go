package allergies

// allergenScores maps each allergen to its bit value
var allergenScores = map[string]uint{
	"eggs":         1,
	"peanuts":      2,
	"shellfish":    4,
	"strawberries": 8,
	"tomatoes":     16,
	"chocolate":    32,
	"pollen":       64,
	"cats":         128,
}

// allergenList maintains the order of allergens for consistent output
var allergenList = []string{
	"eggs",
	"peanuts",
	"shellfish",
	"strawberries",
	"tomatoes",
	"chocolate",
	"pollen",
	"cats",
}

// Allergies returns a list of all allergens that the person is allergic to
// based on the allergy score
func Allergies(score uint) []string {
	result := []string{}
	
	for _, allergen := range allergenList {
		// Check if this allergen's bit is set in the score
		if score&allergenScores[allergen] != 0 {
			result = append(result, allergen)
		}
	}
	
	return result
}

// AllergicTo checks if a person with the given score is allergic to a specific allergen
func AllergicTo(score uint, allergen string) bool {
	allergenValue, ok := allergenScores[allergen]
	if !ok {
		return false
	}
	
	// Check if the allergen's bit is set in the score
	return score&allergenValue != 0
}