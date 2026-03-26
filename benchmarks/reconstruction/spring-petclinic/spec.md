# Spring PetClinic — Reconstruction Spec

## Overview

A veterinary clinic management web application built with Spring Boot. Owners can register, add pets, and schedule visits. Veterinarians are listed with their specialties. The app uses Spring MVC with Thymeleaf templates, Spring Data JPA with H2 in-memory database, and includes caching, validation, and pagination.

## Scope

**IN SCOPE:**
- Entity model hierarchy (BaseEntity, NamedEntity, Person, Owner, Pet, PetType, Visit, Vet, Specialty, Vets)
- Spring Data JPA repositories (OwnerRepository, PetTypeRepository, VetRepository)
- MVC controllers (OwnerController, PetController, VisitController, VetController, WelcomeController)
- PetValidator custom validation
- PetTypeFormatter for form binding
- CacheConfiguration for vet caching
- H2 database schema and seed data
- Application configuration
- Thymeleaf templates for all views

**OUT OF SCOPE:**
- MySQL/PostgreSQL profiles and schemas
- Docker/CI configuration
- Internationalization (messages bundles beyond default)
- CSS/fonts/images (static assets)
- CrashController (error simulation)
- GraalVM native image hints (PetClinicRuntimeHints)
- Actuator customization beyond defaults
- Checkstyle and code formatting configuration

**TARGET:** ~800 LOC Java + ~200 LOC SQL/config + ~300 LOC templates = ~1300 LOC total, ~30 files

## Architecture

- **Language/Runtime:** Java 17
- **Framework:** Spring Boot 4.0.x (Spring 7.x)
- **Key dependencies:**
  - spring-boot-starter-webmvc (Spring MVC)
  - spring-boot-starter-data-jpa (JPA/Hibernate)
  - spring-boot-starter-thymeleaf (templating)
  - spring-boot-starter-validation (Jakarta Bean Validation)
  - spring-boot-starter-cache (Caffeine caching)
  - h2 (in-memory database, runtime scope)
  - jakarta.xml.bind-api (XML binding for Vets JSON/XML endpoint)

### Directory Structure
```
spring-petclinic/
  pom.xml
  src/
    main/
      java/org/springframework/samples/petclinic/
        PetClinicApplication.java
        model/
          BaseEntity.java
          NamedEntity.java
          Person.java
        owner/
          Owner.java
          OwnerController.java
          OwnerRepository.java
          Pet.java
          PetController.java
          PetType.java
          PetTypeFormatter.java
          PetTypeRepository.java
          PetValidator.java
          Visit.java
          VisitController.java
        vet/
          Specialty.java
          Vet.java
          VetController.java
          VetRepository.java
          Vets.java
        system/
          CacheConfiguration.java
          WelcomeController.java
      resources/
        application.properties
        db/h2/
          schema.sql
          data.sql
        templates/
          welcome.html
          error.html
          fragments/
            layout.html
            inputField.html
            selectField.html
          owners/
            findOwners.html
            ownersList.html
            ownerDetails.html
            createOrUpdateOwnerForm.html
          pets/
            createOrUpdatePetForm.html
            createOrUpdateVisitForm.html
          vets/
            vetList.html
```

### Design Patterns
- **Layered architecture:** Controller -> Repository -> Database (no service layer — controllers call repos directly)
- **Mapped superclass hierarchy:** BaseEntity (id) -> NamedEntity (name) -> entities; BaseEntity -> Person (firstName, lastName) -> Owner/Vet
- **Spring Data JPA:** Repository interfaces with derived query methods (no implementation code)
- **Constructor injection:** All controllers use constructor injection (no @Autowired)
- **Form binding with @ModelAttribute:** Controllers use @ModelAttribute methods to pre-load entities before handler methods
- **Cascade persistence:** Owner cascades to Pet, Pet cascades to Visit — saving Owner saves everything

## Data Flow

```
Browser
  |
  v
Spring MVC DispatcherServlet
  |
  v
@Controller (OwnerController, PetController, VisitController, VetController)
  |  - @ModelAttribute methods load entities from DB before handlers
  |  - @InitBinder configures validation and field restrictions
  |  - @Valid triggers Jakarta Bean Validation
  |
  v
Spring Data JPA Repository (OwnerRepository, VetRepository, PetTypeRepository)
  |
  v
H2 In-Memory Database (schema.sql + data.sql loaded at startup)
  |
  v
Thymeleaf Template Engine -> HTML Response
```

## Core Components

### BaseEntity (model/BaseEntity.java)
- **Purpose:** Root of entity hierarchy, provides auto-generated integer ID
- **Superclass:** implements Serializable
- **Annotation:** @MappedSuperclass
- **Fields:** `Integer id` (@Id, @GeneratedValue IDENTITY)
- **Methods:** `getId()`, `setId(Integer)`, `isNew()` (returns true when id is null)

### NamedEntity (model/NamedEntity.java)
- **Purpose:** Adds name field, used by PetType and Specialty
- **Extends:** BaseEntity
- **Annotation:** @MappedSuperclass
- **Fields:** `String name` (@Column, @NotBlank)
- **Methods:** `getName()`, `setName(String)`, `toString()` (returns name or "<null>")

### Person (model/Person.java)
- **Purpose:** Adds first/last name, used by Owner and Vet
- **Extends:** BaseEntity
- **Annotation:** @MappedSuperclass
- **Fields:** `String firstName` (@Column, @NotBlank), `String lastName` (@Column, @NotBlank)

### Owner (owner/Owner.java)
- **Purpose:** Pet owner with contact info and pets collection
- **Extends:** Person
- **Table:** `owners`
- **Fields:**
  - `String address` (@NotBlank)
  - `String city` (@NotBlank)
  - `String telephone` (@NotBlank, @Pattern `\\d{10}`)
  - `List<Pet> pets` (@OneToMany cascade ALL, fetch EAGER, @JoinColumn owner_id, @OrderBy name)
- **Key methods:**
  - `addPet(Pet)` — adds pet only if `pet.isNew()`
  - `getPet(String name)` — find pet by name (case-insensitive)
  - `getPet(String name, boolean ignoreNew)` — optionally skip unsaved pets
  - `getPet(Integer id)` — find pet by ID
  - `addVisit(Integer petId, Visit visit)` — adds visit to specified pet (with null checks via Assert)

### Pet (owner/Pet.java)
- **Purpose:** A pet belonging to an owner
- **Extends:** NamedEntity
- **Table:** `pets`
- **Fields:**
  - `LocalDate birthDate` (@DateTimeFormat "yyyy-MM-dd")
  - `PetType type` (@ManyToOne, @JoinColumn type_id)
  - `Set<Visit> visits` (@OneToMany cascade ALL, fetch EAGER, @JoinColumn pet_id, @OrderBy "date ASC" — LinkedHashSet)
- **Methods:** getters/setters, `addVisit(Visit)`

### PetType (owner/PetType.java)
- **Purpose:** Type of pet (cat, dog, lizard, snake, bird, hamster)
- **Extends:** NamedEntity
- **Table:** `types`

### Visit (owner/Visit.java)
- **Purpose:** A vet visit record
- **Extends:** BaseEntity
- **Table:** `visits`
- **Fields:**
  - `LocalDate date` (@Column visit_date, @DateTimeFormat "yyyy-MM-dd") — defaults to `LocalDate.now()` in constructor
  - `String description` (@NotBlank)

### Vet (vet/Vet.java)
- **Purpose:** Veterinarian with specialties
- **Extends:** Person
- **Table:** `vets`
- **Fields:** `Set<Specialty> specialties` (@ManyToMany fetch EAGER, join table `vet_specialties`)
- **Methods:**
  - `getSpecialties()` — returns sorted list (by name), annotated @XmlElement
  - `getNrOfSpecialties()` — count
  - `addSpecialty(Specialty)`

### Specialty (vet/Specialty.java)
- **Purpose:** Vet specialty (radiology, surgery, dentistry)
- **Extends:** NamedEntity
- **Table:** `specialties`

### Vets (vet/Vets.java)
- **Purpose:** XML/JSON wrapper for vet list
- **Annotation:** @XmlRootElement
- **Fields:** `List<Vet> vets` (lazy-initialized ArrayList)
- **Methods:** `getVetList()` (@XmlElement)

### OwnerRepository (owner/OwnerRepository.java)
- **Extends:** JpaRepository<Owner, Integer>
- **Methods:**
  - `Page<Owner> findByLastNameStartingWith(String lastName, Pageable pageable)`
  - `Optional<Owner> findById(Integer id)`

### PetTypeRepository (owner/PetTypeRepository.java)
- **Extends:** JpaRepository<PetType, Integer>
- **Methods:**
  - `@Query("SELECT ptype FROM PetType ptype ORDER BY ptype.name") List<PetType> findPetTypes()`

### VetRepository (vet/VetRepository.java)
- **Extends:** Repository<Vet, Integer> (NOT JpaRepository)
- **Methods:**
  - `Collection<Vet> findAll()` — @Transactional(readOnly=true), @Cacheable("vets")
  - `Page<Vet> findAll(Pageable pageable)` — @Transactional(readOnly=true), @Cacheable("vets")

### OwnerController (owner/OwnerController.java)
- **Endpoints:**
  - `GET /owners/new` -> createOrUpdateOwnerForm
  - `POST /owners/new` -> validate, save, redirect to `/owners/{id}`
  - `GET /owners/find` -> findOwners search form
  - `GET /owners?page=1` -> search by lastName (pagination, 5 per page)
    - 0 results: show error on search form
    - 1 result: redirect to owner detail
    - multiple: show paginated list
  - `GET /owners/{ownerId}` -> owner detail page (ModelAndView)
  - `GET /owners/{ownerId}/edit` -> edit form
  - `POST /owners/{ownerId}/edit` -> validate, check ID mismatch, save, redirect
- **@ModelAttribute("owner"):** loads owner from DB by ownerId path variable, or returns new Owner() if no ID
- **@InitBinder:** disallows "id" field binding

### PetController (owner/PetController.java)
- **Base path:** `/owners/{ownerId}`
- **Endpoints:**
  - `GET /pets/new` -> createOrUpdatePetForm (adds new Pet to owner)
  - `POST /pets/new` -> validate (duplicate name check, future birth date check), save via owner cascade
  - `GET /pets/{petId}/edit` -> edit form
  - `POST /pets/{petId}/edit` -> validate, update existing pet properties, save
- **@ModelAttribute("types"):** loads all PetType from PetTypeRepository
- **@ModelAttribute("owner"):** loads owner by ownerId
- **@ModelAttribute("pet"):** loads pet from owner by petId, or returns new Pet()
- **Validation:** PetValidator (custom, set via @InitBinder)

### PetValidator (owner/PetValidator.java)
- **Purpose:** Custom validator for Pet form
- **Rules:** name must not be blank, type must be selected, birthDate must not be null
- **Implements:** org.springframework.validation.Validator
- **supports():** Pet.class only

### PetTypeFormatter (owner/PetTypeFormatter.java)
- **Purpose:** Converts between PetType objects and their string names for form select fields
- **Implements:** Formatter<PetType>
- **print():** returns petType.getName()
- **parse():** looks up PetType by name from PetTypeRepository.findPetTypes(), throws ParseException if not found

### VisitController (owner/VisitController.java)
- **Endpoints:**
  - `GET /owners/{ownerId}/pets/{petId}/visits/new` -> createOrUpdateVisitForm
  - `POST /owners/{ownerId}/pets/{petId}/visits/new` -> validate, add visit to pet via owner.addVisit(), save owner
- **@ModelAttribute("visit"):** loads owner, gets pet, creates new Visit, adds it to pet, puts pet+owner in model

### VetController (vet/VetController.java)
- **Endpoints:**
  - `GET /vets.html?page=1` -> paginated vet list (5 per page), Thymeleaf view
  - `GET /vets` -> JSON response (@ResponseBody) of all vets as Vets wrapper object

### WelcomeController (system/WelcomeController.java)
- **Endpoint:** `GET /` -> welcome view

### CacheConfiguration (system/CacheConfiguration.java)
- **Purpose:** Configures Caffeine cache for vets
- **Cache name:** "vets"
- **Settings:** max 200 entries, 60 second TTL

### PetClinicApplication
- **Purpose:** @SpringBootApplication main class
- **Method:** `main(String[])` calls `SpringApplication.run()`

## Configuration

### application.properties
```properties
database=h2
spring.sql.init.schema-locations=classpath*:db/${database}/schema.sql
spring.sql.init.data-locations=classpath*:db/${database}/data.sql
spring.thymeleaf.mode=HTML
spring.jpa.hibernate.ddl-auto=none
spring.jpa.open-in-view=false
spring.jpa.hibernate.naming.physical-strategy=org.hibernate.boot.model.naming.PhysicalNamingStrategySnakeCaseImpl
spring.jpa.properties.hibernate.default_batch_fetch_size=16
management.endpoints.web.exposure.include=*
logging.level.org.springframework=INFO
spring.web.resources.cache.cachecontrol.max-age=12h
```

## Database Schema

### Tables
7 tables: `vets`, `specialties`, `vet_specialties`, `types`, `owners`, `pets`, `visits`

### Schema (H2)
```sql
CREATE TABLE vets (
  id         INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  first_name VARCHAR(30),
  last_name  VARCHAR(30)
);

CREATE TABLE specialties (
  id   INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  name VARCHAR(80)
);

CREATE TABLE vet_specialties (
  vet_id       INTEGER NOT NULL REFERENCES vets(id),
  specialty_id INTEGER NOT NULL REFERENCES specialties(id)
);

CREATE TABLE types (
  id   INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  name VARCHAR(80)
);

CREATE TABLE owners (
  id         INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  first_name VARCHAR(30),
  last_name  VARCHAR_IGNORECASE(30),
  address    VARCHAR(255),
  city       VARCHAR(80),
  telephone  VARCHAR(20)
);

CREATE TABLE pets (
  id         INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  name       VARCHAR(30),
  birth_date DATE,
  type_id    INTEGER NOT NULL REFERENCES types(id),
  owner_id   INTEGER REFERENCES owners(id)
);

CREATE TABLE visits (
  id          INTEGER GENERATED BY DEFAULT AS IDENTITY PRIMARY KEY,
  pet_id      INTEGER REFERENCES pets(id),
  visit_date  DATE,
  description VARCHAR(255)
);
```

### Seed Data
- 6 vets (James Carter, Helen Leary, Linda Douglas, Rafael Ortega, Henry Stevens, Sharon Jenkins)
- 3 specialties (radiology, surgery, dentistry)
- 5 vet_specialty assignments
- 6 pet types (cat, dog, lizard, snake, bird, hamster)
- 10 owners with addresses and phone numbers
- 13 pets assigned to owners
- 4 visits (rabies shots, neutered, spayed)

## Test Cases

### Functional Tests
1. **Find owner by last name:** GET /owners?lastName=Davis -> returns page with Betty Davis and Harold Davis
2. **Create owner:** POST /owners/new with valid data -> redirects to /owners/{id}, owner persisted
3. **Create owner validation:** POST /owners/new with blank fields -> returns form with errors
4. **View owner detail:** GET /owners/1 -> shows George Franklin with pet Leo
5. **Add pet to owner:** POST /owners/1/pets/new with name="Buddy", type=dog, birthDate=2020-01-01 -> pet added
6. **Duplicate pet name:** POST /owners/1/pets/new with name="Leo" -> validation error "already exists"
7. **Future birth date:** POST /owners/1/pets/new with birthDate in future -> validation error
8. **Add visit:** POST /owners/1/pets/1/visits/new with description="checkup" -> visit added with today's date
9. **List vets HTML:** GET /vets.html -> paginated list showing James Carter (no specialties), Helen Leary (radiology)
10. **List vets JSON:** GET /vets -> JSON response with all 6 vets and their specialties

### Edge Cases
1. **Owner not found:** GET /owners/999 -> IllegalArgumentException
2. **Empty search:** GET /owners (no lastName param) -> returns all owners paginated
3. **Single result search:** GET /owners?lastName=Franklin -> redirects directly to George Franklin's page
4. **Edit owner ID mismatch:** POST /owners/1/edit with form containing different owner ID -> error

## Build & Run

### Build
```bash
./mvnw clean package -DskipTests
```

### Run
```bash
./mvnw spring-boot:run
# App starts on http://localhost:8080
```

### Test
```bash
./mvnw test
```

### Expected Behavior at Runtime
- `http://localhost:8080/` -> Welcome page
- `http://localhost:8080/owners/find` -> Owner search form
- `http://localhost:8080/owners?lastName=` -> All owners (paginated)
- `http://localhost:8080/vets.html` -> Vet list (paginated)
- `http://localhost:8080/vets` -> JSON: `{"vetList":[{"id":1,"firstName":"James","lastName":"Carter","specialties":[],...}]}`

## Acceptance Criteria

1. Project builds with `mvn clean package` without errors
2. Application starts on port 8080 with H2 database
3. Welcome page renders at GET /
4. Owner CRUD works: create, view, edit, search with pagination (5 per page)
5. Pet CRUD works: add pet to owner, edit pet, duplicate name validation, future birthdate validation
6. Visit creation works: add visit to pet with auto-dated visit date
7. Vet listing works: HTML paginated view at /vets.html, JSON at /vets
8. Vet specialties display correctly (sorted by name)
9. Cache is configured for vets (Caffeine, max 200, 60s TTL)
10. Seed data loads: 10 owners, 13 pets, 6 vets, 4 visits
11. Entity inheritance hierarchy: BaseEntity -> NamedEntity/Person -> concrete entities
12. All repositories use Spring Data JPA (no manual implementations)
13. PetTypeFormatter correctly converts between PetType and String for form binding
14. Owner.addVisit(petId, visit) correctly delegates to the right Pet
