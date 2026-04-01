class Allergies:
    ALLERGENS = {
        "eggs": 1,
        "peanuts": 2,
        "shellfish": 4,
        "strawberries": 8,
        "tomatoes": 16,
        "chocolate": 32,
        "pollen": 64,
        "cats": 128,
    }

    def __init__(self, score):
        self.score = score

    def allergic_to(self, item):
        allergen_score = self.ALLERGENS.get(item)
        if allergen_score is None:
            return False
        return (self.score & allergen_score) == allergen_score

    @property
    def lst(self):
        return [allergen for allergen in self.ALLERGENS if self.allergic_to(allergen)]