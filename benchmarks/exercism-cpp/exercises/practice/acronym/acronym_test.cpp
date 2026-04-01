#include "acronym.h"
#include "test/catch.hpp"

using namespace std;

TEST_CASE("basic") {
    const string actual = acronym::generate_acronym("Portable Network Graphics");

    const string expected{"PNG"};

    REQUIRE(expected == actual);
}

#if defined(EXERCISM_RUN_ALL_TESTS)
TEST_CASE("lowercase_words") {
    const string actual = acronym::generate_acronym("Ruby on Rails");

    const string expected{"ROR"};

    REQUIRE(expected == actual);
}

TEST_CASE("punctuation") {
    const string actual = acronym::generate_acronym("First In, First Out");

    const string expected{"FIFO"};

    REQUIRE(expected == actual);
}

TEST_CASE("all_caps_word") {
    const string actual = acronym::generate_acronym("GNU Image Manipulation Program");

    const string expected{"GIMP"};

    REQUIRE(expected == actual);
}

TEST_CASE("punctuation_without_whitespace") {
    const string actual =
        acronym::generate_acronym("Complementary metal-oxide semiconductor");

    const string expected{"CMOS"};

    REQUIRE(expected == actual);
}

TEST_CASE("very_long_abbreviation") {
    const string actual = acronym::generate_acronym(
        "Rolling On The Floor Laughing So Hard That My Dogs Came Over And "
        "Licked Me");

    const string expected{"ROTFLSHTMDCOALM"};

    REQUIRE(expected == actual);
}

TEST_CASE("consecutive_delimiters") {
    const string actual =
        acronym::generate_acronym("Something - I made up from thin air");

    const string expected{"SIMUFTA"};

    REQUIRE(expected == actual);
}

TEST_CASE("apostrophes") {
    const string actual = acronym::generate_acronym("Halley's Comet");

    const string expected{"HC"};

    REQUIRE(expected == actual);
}

TEST_CASE("underscore_emphasis") {
    const string actual = acronym::generate_acronym("The Road _Not_ Taken");

    const string expected{"TRNT"};

    REQUIRE(expected == actual);
}
#endif
