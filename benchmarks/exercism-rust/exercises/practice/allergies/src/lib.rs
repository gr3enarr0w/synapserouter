use std::cmp::PartialEq;

#[derive(Debug, PartialEq, Clone, Copy, Eq, PartialOrd, Ord)]
pub enum Allergen {
    Eggs = 1,
    Peanuts = 2,
    Shellfish = 4,
    Strawberries = 8,
    Tomatoes = 16,
    Chocolate = 32,
    Pollen = 64,
    Cats = 128,
}

pub struct Allergies {
    score: u32,
}

impl Allergies {
    pub fn new(score: u32) -> Self {
        Allergies { score }
    }

    pub fn is_allergic_to(&self, allergen: &Allergen) -> bool {
        (self.score & (*allergen as u32)) != 0
    }

    pub fn allergies(&self) -> Vec<Allergen> {
        let mut result = Vec::new();
        
        // Check each allergen
        if self.is_allergic_to(&Allergen::Eggs) {
            result.push(Allergen::Eggs);
        }
        if self.is_allergic_to(&Allergen::Peanuts) {
            result.push(Allergen::Peanuts);
        }
        if self.is_allergic_to(&Allergen::Shellfish) {
            result.push(Allergen::Shellfish);
        }
        if self.is_allergic_to(&Allergen::Strawberries) {
            result.push(Allergen::Strawberries);
        }
        if self.is_allergic_to(&Allergen::Tomatoes) {
            result.push(Allergen::Tomatoes);
        }
        if self.is_allergic_to(&Allergen::Chocolate) {
            result.push(Allergen::Chocolate);
        }
        if self.is_allergic_to(&Allergen::Pollen) {
            result.push(Allergen::Pollen);
        }
        if self.is_allergic_to(&Allergen::Cats) {
            result.push(Allergen::Cats);
        }
        
        result
    }
}