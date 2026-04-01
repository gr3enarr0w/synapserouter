const ALLERGENS = [
  { name: 'eggs', score: 1 },
  { name: 'peanuts', score: 2 },
  { name: 'shellfish', score: 4 },
  { name: 'strawberries', score: 8 },
  { name: 'tomatoes', score: 16 },
  { name: 'chocolate', score: 32 },
  { name: 'pollen', score: 64 },
  { name: 'cats', score: 128 },
];

export class Allergies {
  constructor(score) {
    this.score = score;
  }

  allergicTo(allergenName) {
    const allergen = ALLERGENS.find(a => a.name === allergenName);
    if (!allergen) {
      return false;
    }
    return (this.score & allergen.score) !== 0;
  }

  list() {
    return ALLERGENS
      .filter(allergen => (this.score & allergen.score) !== 0)
      .map(allergen => allergen.name);
  }
}