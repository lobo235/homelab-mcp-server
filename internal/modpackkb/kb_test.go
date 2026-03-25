package modpackkb

import (
	"log/slog"
	"os"
	"testing"
)

func testKB(t *testing.T) *KB {
	t.Helper()
	dir := t.TempDir()
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	kb, err := New(dir, log)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return kb
}

func seedEntry(t *testing.T, kb *KB, slug, name string, cfID int) {
	t.Helper()
	mk := &ModpackKnowledge{
		Slug:          slug,
		Name:          name,
		CurseForgeID:  cfID,
		ModloaderType: "forge",
		HasServerPack: true,
		Source:        "test",
		UpdatedBy:     "test",
	}
	if err := kb.Save(mk); err != nil {
		t.Fatalf("Save seed %q: %v", slug, err)
	}
}

func TestSlugify(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"All the Mods 9", "all-the-mods-9"},
		{"All the Mods 9 - To the Sky", "all-the-mods-9-to-the-sky"},
		{"  RLCraft  ", "rlcraft"},
		{"ATM10", "atm10"},
		{"My---Pack", "my-pack"},
		{"Pack (Beta)", "pack-beta"},
	}

	for _, tt := range tests {
		got := Slugify(tt.input)
		if got != tt.want {
			t.Errorf("Slugify(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSaveAndGet(t *testing.T) {
	kb := testKB(t)

	mk := &ModpackKnowledge{
		Slug:          "all-the-mods-9",
		Name:          "All the Mods 9",
		CurseForgeID:  715572,
		ModloaderType: "forge",
		HasServerPack: true,
		Source:        "manual",
		UpdatedBy:     "test",
	}

	if err := kb.Save(mk); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := kb.Get("all-the-mods-9")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "All the Mods 9" {
		t.Errorf("Name = %q, want %q", got.Name, "All the Mods 9")
	}
	if got.CurseForgeID != 715572 {
		t.Errorf("CurseForgeID = %d, want %d", got.CurseForgeID, 715572)
	}
	if got.CreatedAt == "" {
		t.Error("CreatedAt should be set")
	}
	if got.UpdatedAt == "" {
		t.Error("UpdatedAt should be set")
	}
}

func TestSaveMerge(t *testing.T) {
	kb := testKB(t)

	// Save initial entry.
	mk := &ModpackKnowledge{
		Slug:          "all-the-mods-9",
		Name:          "All the Mods 9",
		ModloaderType: "forge",
		InitMemory:    "4G",
		MaxMemory:     "8G",
		Source:        "manual",
		UpdatedBy:     "test",
	}
	if err := kb.Save(mk); err != nil {
		t.Fatalf("Save initial: %v", err)
	}

	// Save update — only change MaxMemory and add CurseForgeID.
	update := &ModpackKnowledge{
		Slug:         "all-the-mods-9",
		CurseForgeID: 715572,
		MaxMemory:    "10G",
		Source:       "conversation",
		UpdatedBy:    "claude",
	}
	if err := kb.Save(update); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	got, err := kb.Get("all-the-mods-9")
	if err != nil {
		t.Fatalf("Get after merge: %v", err)
	}

	// Merged fields.
	if got.CurseForgeID != 715572 {
		t.Errorf("CurseForgeID = %d, want 715572", got.CurseForgeID)
	}
	if got.MaxMemory != "10G" {
		t.Errorf("MaxMemory = %q, want %q", got.MaxMemory, "10G")
	}
	// Preserved fields.
	if got.Name != "All the Mods 9" {
		t.Errorf("Name = %q, want %q (preserved)", got.Name, "All the Mods 9")
	}
	if got.ModloaderType != "forge" {
		t.Errorf("ModloaderType = %q, want %q (preserved)", got.ModloaderType, "forge")
	}
	if got.InitMemory != "4G" {
		t.Errorf("InitMemory = %q, want %q (preserved)", got.InitMemory, "4G")
	}
}

func TestSaveVersionMerge(t *testing.T) {
	kb := testKB(t)

	// Save with one version.
	mk := &ModpackKnowledge{
		Slug:   "atm9",
		Name:   "ATM9",
		Source: "test",
		Versions: []VersionKnowledge{
			{PackVersion: "1.0.0", JavaVersion: 17, Source: "test"},
		},
	}
	if err := kb.Save(mk); err != nil {
		t.Fatalf("Save initial: %v", err)
	}

	// Update: overwrite existing version and add new one.
	update := &ModpackKnowledge{
		Slug:   "atm9",
		Source: "test",
		Versions: []VersionKnowledge{
			{PackVersion: "1.0.0", JavaVersion: 21, Source: "test-updated"},
			{PackVersion: "2.0.0", JavaVersion: 21, Source: "test"},
		},
	}
	if err := kb.Save(update); err != nil {
		t.Fatalf("Save update: %v", err)
	}

	got, err := kb.Get("atm9")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	if len(got.Versions) != 2 {
		t.Fatalf("Versions count = %d, want 2", len(got.Versions))
	}
	if got.Versions[0].JavaVersion != 21 {
		t.Errorf("Version 1.0.0 JavaVersion = %d, want 21 (overwritten)", got.Versions[0].JavaVersion)
	}
	if got.Versions[1].PackVersion != "2.0.0" {
		t.Errorf("Version[1] = %q, want %q", got.Versions[1].PackVersion, "2.0.0")
	}
}

func TestSaveAutoSlug(t *testing.T) {
	kb := testKB(t)

	mk := &ModpackKnowledge{
		Name:   "All the Mods 10",
		Source: "test",
	}
	if err := kb.Save(mk); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := kb.Get("all-the-mods-10")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Slug != "all-the-mods-10" {
		t.Errorf("Slug = %q, want %q", got.Slug, "all-the-mods-10")
	}
}

func TestGetNotFound(t *testing.T) {
	kb := testKB(t)
	_, err := kb.Get("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent slug")
	}
}

func TestGetByName(t *testing.T) {
	kb := testKB(t)
	seedEntry(t, kb, "all-the-mods-9", "All the Mods 9", 715572)
	seedEntry(t, kb, "all-the-mods-9-to-the-sky", "All the Mods 9 - To the Sky", 715573)

	tests := []struct {
		query    string
		wantSlug string
	}{
		{"All the Mods 9", "all-the-mods-9"},
		{"ATM9", "all-the-mods-9"},
		{"atm9", "all-the-mods-9"},
		{"atm9 to the sky", "all-the-mods-9-to-the-sky"},
		{"ATM9 To the Sky", "all-the-mods-9-to-the-sky"},
	}

	for _, tt := range tests {
		got, err := kb.GetByName(tt.query)
		if err != nil {
			t.Errorf("GetByName(%q): %v", tt.query, err)
			continue
		}
		if got.Slug != tt.wantSlug {
			t.Errorf("GetByName(%q).Slug = %q, want %q", tt.query, got.Slug, tt.wantSlug)
		}
	}
}

func TestGetByNameNotFound(t *testing.T) {
	kb := testKB(t)
	_, err := kb.GetByName("nonexistent pack")
	if err == nil {
		t.Fatal("expected error for nonexistent name")
	}
}

func TestGetByCurseForgeID(t *testing.T) {
	kb := testKB(t)
	seedEntry(t, kb, "all-the-mods-9", "All the Mods 9", 715572)

	got, err := kb.GetByCurseForgeID(715572)
	if err != nil {
		t.Fatalf("GetByCurseForgeID: %v", err)
	}
	if got.Slug != "all-the-mods-9" {
		t.Errorf("Slug = %q, want %q", got.Slug, "all-the-mods-9")
	}
}

func TestGetByCurseForgeIDNotFound(t *testing.T) {
	kb := testKB(t)
	_, err := kb.GetByCurseForgeID(999999)
	if err == nil {
		t.Fatal("expected error for nonexistent CF ID")
	}
}

func TestDelete(t *testing.T) {
	kb := testKB(t)
	seedEntry(t, kb, "all-the-mods-9", "All the Mods 9", 715572)

	if err := kb.Delete("all-the-mods-9"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	_, err := kb.Get("all-the-mods-9")
	if err == nil {
		t.Fatal("expected error after delete")
	}
}

func TestDeleteNotFound(t *testing.T) {
	kb := testKB(t)
	err := kb.Delete("nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent slug")
	}
}

func TestList(t *testing.T) {
	kb := testKB(t)
	seedEntry(t, kb, "all-the-mods-9", "All the Mods 9", 715572)
	seedEntry(t, kb, "all-the-mods-10", "All the Mods 10", 855888)

	entries, err := kb.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("List count = %d, want 2", len(entries))
	}
}

func TestListEmpty(t *testing.T) {
	kb := testKB(t)
	entries, err := kb.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List count = %d, want 0", len(entries))
	}
}

func TestSearch(t *testing.T) {
	kb := testKB(t)
	seedEntry(t, kb, "all-the-mods-9", "All the Mods 9", 715572)
	seedEntry(t, kb, "all-the-mods-10", "All the Mods 10", 855888)
	seedEntry(t, kb, "rlcraft", "RLCraft", 285109)

	tests := []struct {
		query string
		want  int
	}{
		{"all the mods", 2},
		{"atm", 2},
		{"rlcraft", 1},
		{"nonexistent", 0},
	}

	for _, tt := range tests {
		got, err := kb.Search(tt.query)
		if err != nil {
			t.Errorf("Search(%q): %v", tt.query, err)
			continue
		}
		if len(got) != tt.want {
			t.Errorf("Search(%q) = %d results, want %d", tt.query, len(got), tt.want)
		}
	}
}

func TestExpandAbbreviation(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"atm9", "all-the-mods-9"},
		{"atm10", "all-the-mods-10"},
		{"atm9 to the sky", "all-the-mods-9-to-the-sky"},
		{"atm", "all-the-mods"},
		{"rlcraft", "rlcraft"}, // no expansion
	}

	for _, tt := range tests {
		got := expandAbbreviation(tt.input)
		if got != tt.want {
			t.Errorf("expandAbbreviation(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSaveRequiresSlugOrName(t *testing.T) {
	kb := testKB(t)
	err := kb.Save(&ModpackKnowledge{})
	if err == nil {
		t.Fatal("expected error when saving with no slug or name")
	}
}
