#include "acronym.h"
#include <cctype>
#include <sstream>

using namespace std;

namespace acronym {
    string generate_acronym(const string& phrase) {
        string result;
        bool newWord = true;
        
        for (char c : phrase) {
            if (isalpha(c)) {
                if (newWord) {
                    result += toupper(c);
                    newWord = false;
                }
            } else if (c == ' ' || c == '-' || c == '_') {
                // These characters explicitly indicate a new word
                newWord = true;
            }
            // For apostrophes and other punctuation, don't change newWord state
            // so they don't trigger new words but are also not included
        }
        
        return result;
    }
}
