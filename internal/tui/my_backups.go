package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"gopkg.in/yaml.v3"

	"github.com/nassiharel/clim/internal/manifest"
)

// backupFileInfo describes one backup file in the backups directory.
type backupFileInfo struct {
	name      string    // filename without path
	path      string    // full path
	modTime   time.Time // file modification time
	toolCount int       // number of tools in the manifest
}

// backupDeletedMsg is sent after a backup file is deleted.
type backupDeletedMsg struct {
	name string
	err  error
}

// scanBackupsDir lists YAML files in the backups directory, sorted newest first.
func scanBackupsDir() []backupFileInfo {
	bdir, err := backupsDir()
	if err != nil {
		return nil
	}

	entries, err := os.ReadDir(bdir)
	if err != nil {
		return nil
	}

	var files []backupFileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}

		fullPath := filepath.Join(bdir, name)
		info, err := e.Info()
		if err != nil {
			continue
		}

		// Parse tool count.
		toolCount := 0
		if data, err := os.ReadFile(fullPath); err == nil {
			var m manifest.Manifest
			if err := yaml.Unmarshal(data, &m); err == nil {
				toolCount = len(m.Tools)
			}
		}

		files = append(files, backupFileInfo{
			name:      name,
			path:      fullPath,
			modTime:   info.ModTime(),
			toolCount: toolCount,
		})
	}

	// Sort newest first.
	sort.Slice(files, func(i, j int) bool {
		return files[i].modTime.After(files[j].modTime)
	})

	return files
}

// --- Key handling ---

func (m Model) handleKeyMyBackups(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", "q", "backspace":
		m.viewingMyBackups = false
		m.statusMsg = ""
		return m, nil
	case "up", "k":
		if m.myBackupsCursor > 0 {
			m.myBackupsCursor--
		}
	case "down", "j":
		if m.myBackupsCursor < len(m.myBackupFiles)-1 {
			m.myBackupsCursor++
		}
	case "enter":
		// Import selected backup.
		if m.myBackupsCursor < len(m.myBackupFiles) {
			path := m.myBackupFiles[m.myBackupsCursor].path
			m.viewingMyBackups = false
			m.backupItems = nil
			m.backupDone = 0
			m.backupMode = backupModeImport
			m.activeTab = tabBackup
			m.cursor = 0
			m.statusMsg = "Building import plan..."
			return m, buildImportPlanCmd(m.svc, path)
		}
	case "d":
		// Delete selected backup.
		if m.myBackupsCursor < len(m.myBackupFiles) {
			file := m.myBackupFiles[m.myBackupsCursor]
			return m, deleteBackupFileCmd(file.name, file.path)
		}
	}
	return m, nil
}

// --- Commands ---

func deleteBackupFileCmd(name, path string) tea.Cmd {
	return func() tea.Msg {
		err := os.Remove(path)
		return backupDeletedMsg{name: name, err: err}
	}
}

// --- Rendering ---

func (m Model) renderMyBackupsView() string {
	var b strings.Builder

	b.WriteString("\n  " + detailTitleStyle.Render("My Backups") + "\n\n")

	if len(m.myBackupFiles) == 0 {
		b.WriteString("  " + dimVersion.Render("No backups yet. Use Export to create one.") + "\n")
		b.WriteString("\n  " + dimVersion.Render("Esc") + " back\n")
		return b.String()
	}

	// Header.
	b.WriteString("  " +
		headerStyle.Render(fixedWidth("FILE", colName)) + "  " +
		headerStyle.Render(fixedWidth("TOOLS", colPackTools)) + "  " +
		headerStyle.Render("DATE") + "\n")

	visibleRows := m.height - 12
	if visibleRows < 3 {
		visibleRows = 3
	}
	start := 0
	if m.myBackupsCursor >= visibleRows {
		start = m.myBackupsCursor - visibleRows + 1
	}

	for i := start; i < len(m.myBackupFiles) && i < start+visibleRows; i++ {
		file := m.myBackupFiles[i]
		cursor := "  "
		if i == m.myBackupsCursor {
			cursor = "▸ "
		}

		toolCount := fmt.Sprintf("%d", file.toolCount)
		date := file.modTime.Format("2006-01-02 15:04")

		line := cursor +
			nameStyle.Render(fixedWidth(file.name, colName)) + "  " +
			dimVersion.Render(fixedWidth(toolCount, colPackTools)) + "  " +
			dimVersion.Render(date)

		if i == m.myBackupsCursor {
			w := lipgloss.Width(line)
			if w < m.width {
				line += strings.Repeat(" ", m.width-w)
			}
			line = selectedRowStyle.Render(line)
		}
		b.WriteString(line + "\n")
	}

	// Pad.
	rendered := min(len(m.myBackupFiles)-start, visibleRows)
	for range max(visibleRows-rendered, 0) {
		b.WriteString("\n")
	}

	b.WriteString("\n  " + dimVersion.Render("Enter") + " import   " +
		dimVersion.Render("d") + " delete   " +
		dimVersion.Render("Esc") + " back\n")
	return b.String()
}
