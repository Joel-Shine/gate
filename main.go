package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/sahilm/fuzzy"
)

var (
	historyFile = filepath.Join(os.Getenv("HOME"), ".g_history.txt")
	scoreFile   = filepath.Join(os.Getenv("HOME"), ".g_scores.json")
	skipDirs    = map[string]bool{".git": true, "node_modules": true, "windows": true, "program files": true, "$recycle.bin": true, "venv": true, ".venv": true}
	binaryExts  = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".mp4": true, ".mp3": true, ".zip": true, ".pdf": true, ".exe": true}
)

type PathData struct {
	AbsPath string
	IsDir   bool
}

type Match struct {
	RelPath string
	AbsPath string
	Score   int
	Visits  int
	IsDir   bool
}

func saveHistory(path string) {
	os.WriteFile(historyFile, []byte(path), 0644)
}

func loadScores() map[string]int {
	scores := make(map[string]int)
	data, err := os.ReadFile(scoreFile)
	if err == nil {
		json.Unmarshal(data, &scores)
	}
	return scores
}

func saveScore(path string) {
	scores := loadScores()
	scores[path]++
	data, _ := json.Marshal(scores)
	os.WriteFile(scoreFile, data, 0644)
}

func getPeekString(path string, isDir bool) string {
	if !isDir {
		info, err := os.Stat(path)
		if err != nil {
			return "⚠️ (Error)"
		}
		size := info.Size()
		if size < 1024 {
			return fmt.Sprintf("%d B", size)
		} else if size < 1024*1024 {
			return fmt.Sprintf("%.1f KB", float64(size)/1024.0)
		}
		return fmt.Sprintf("%.1f MB", float64(size)/(1024.0*1024.0))
	}

	entries, err := os.ReadDir(path)
	if err != nil {
		return "🔒 (Access Denied)"
	}

	var visible []os.DirEntry
	for _, e := range entries {
		if !strings.HasPrefix(e.Name(), ".") {
			visible = append(visible, e)
		}
	}

	if len(visible) == 0 {
		return "∅ (Empty)"
	}

	sort.Slice(visible, func(i, j int) bool {
		if visible[i].IsDir() != visible[j].IsDir() {
			return visible[i].IsDir()
		}
		return strings.ToLower(visible[i].Name()) < strings.ToLower(visible[j].Name())
	})

	var parts []string
	maxItems := 4
	for i, e := range visible {
		if i >= maxItems {
			break
		}
		if e.IsDir() {
			parts = append(parts, "📁 "+e.Name())
		} else {
			parts = append(parts, "📄 "+e.Name())
		}
	}

	res := strings.Join(parts, ", ")
	if len(visible) > maxItems {
		res += ", ..."
	}
	return res
}

func getPaths(basePath string, maxDepth int, allowedExts map[string]bool) map[string]PathData {
	paths := make(map[string]PathData)
	baseDepth := strings.Count(basePath, string(os.PathSeparator))

	filepath.WalkDir(basePath, func(path string, d os.DirEntry, err error) error {
		if err != nil || path == basePath {
			return nil
		}

		currentDepth := strings.Count(path, string(os.PathSeparator)) - baseDepth
		if currentDepth > maxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}

		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}

		if d.IsDir() {
			if skipDirs[strings.ToLower(d.Name())] {
				return filepath.SkipDir
			}
			// If we are filtering by extension, we don't add directories to the search results
			if allowedExts == nil {
				relPath, _ := filepath.Rel(basePath, path)
				paths[strings.ReplaceAll(relPath, "\\", "/")] = PathData{AbsPath: path, IsDir: true}
			}
		} else {
			ext := strings.ToLower(filepath.Ext(d.Name()))
			if allowedExts != nil && !allowedExts[ext] {
				return nil // Skip files that don't match our -e flag
			}
			relPath, _ := filepath.Rel(basePath, path)
			paths[strings.ReplaceAll(relPath, "\\", "/")] = PathData{AbsPath: path, IsDir: false}
		}
		return nil
	})

	return paths
}

func getContentMatches(basePath, targetText string, maxDepth int, allowedExts map[string]bool) []Match {
	var matches []Match
	var mu sync.Mutex
	var wg sync.WaitGroup
	targetLower := strings.ToLower(targetText)
	scoresDB := loadScores()

	var scan func(currentPath string, currentDepth int)
	scan = func(currentPath string, currentDepth int) {
		defer wg.Done()
		if currentDepth > maxDepth {
			return
		}

		entries, err := os.ReadDir(currentPath)
		if err != nil {
			return
		}

		for _, entry := range entries {
			if entry.Type()&os.ModeSymlink != 0 {
				continue
			}

			absPath := filepath.Join(currentPath, entry.Name())

			if entry.IsDir() {
				if skipDirs[strings.ToLower(entry.Name())] {
					continue
				}
				wg.Add(1)
				go scan(absPath, currentDepth+1)
			} else {
				ext := strings.ToLower(filepath.Ext(entry.Name()))
				
				if allowedExts != nil {
					if !allowedExts[ext] {
						continue
					}
				} else if binaryExts[ext] {
					continue
				}

				info, err := entry.Info()
				if err != nil || info.Size() > 5*1024*1024 {
					continue
				}

				wg.Add(1)
				go func(p string) {
					defer wg.Done()
					data, err := os.ReadFile(p)
					if err == nil && strings.Contains(strings.ToLower(string(data)), targetLower) {
						relPath, _ := filepath.Rel(basePath, p)
						relPath = strings.ReplaceAll(relPath, "\\", "/")
						visits := scoresDB[p]
						bonus := visits * 2
						if bonus > 20 {
							bonus = 20
						}

						mu.Lock()
						matches = append(matches, Match{
							RelPath: relPath,
							AbsPath: p,
							Score:   100 + bonus,
							Visits:  visits,
							IsDir:   false,
						})
						mu.Unlock()
					}
				}(absPath)
			}
		}
	}

	wg.Add(1)
	scan(basePath, 1)
	wg.Wait()
	return matches
}

func printHelp() {
	re := lipgloss.NewRenderer(os.Stderr)
	headerStyle := re.NewStyle().Foreground(lipgloss.Color("14")).Bold(true).Padding(0, 1)
	cellStyle := re.NewStyle().Padding(0, 1)

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(re.NewStyle().Foreground(lipgloss.Color("10"))).
		Headers("Command Flag", "What it does", "Example Usage").
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return headerStyle
			}
			return cellStyle
		})

	t.Row("g <target>", "Fuzzy search files & folders", "g config")
	t.Row("g <target> <depth>", "Fuzzy search with a custom depth limit", "g project 8")
	t.Row("g -e <exts> <target>", "Filter by file extensions (comma separated)", "g -e html,css,js index")
	t.Row("g -c <text>", "Search inside file contents for a string", "g -c auth")
	t.Row("g -c -e <exts> <text>", "Content search within specific extensions", "g -c -e go,py main")
	t.Row("g back", "Jump seamlessly to your previous directory", "g back")
	t.Row("g ..", "Go up to the parent directory", "g ..")

	fmt.Fprintln(os.Stderr, lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Bold(true).Render("\n🚀 g-cli: The Blazing Fast Navigator"))
	fmt.Fprintln(os.Stderr, t.Render())
	fmt.Fprintln(os.Stderr, "")
}

// Safely truncates strings containing emojis and unicode
func truncate(text string, maxLen int) string {
	runes := []rune(text)
	if len(runes) > maxLen {
		return string(runes[:maxLen-3]) + "..."
	}
	return text
}

func main() {
	args := os.Args[1:]

	if len(args) == 0 || args[0] == "-help" || args[0] == "--help" || args[0] == "-h" {
		printHelp()
		return
	}

	contentSearch := false
	var allowedExts map[string]bool

	// Parse chained flags dynamically
	for len(args) > 0 {
		if args[0] == "-c" {
			contentSearch = true
			args = args[1:]
		} else if args[0] == "-e" && len(args) > 1 {
			allowedExts = make(map[string]bool)
			for _, ext := range strings.Split(args[1], ",") {
				ext = strings.ToLower(strings.TrimSpace(ext))
				if !strings.HasPrefix(ext, ".") {
					ext = "." + ext
				}
				allowedExts[ext] = true
			}
			args = args[2:]
		} else {
			break
		}
	}

	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, lipgloss.NewStyle().Foreground(lipgloss.Color("9")).Bold(true).Render("Missing search target! Type 'g -help' for usage."))
		return
	}

	target := args[0]
	searchDepth := 5

	if len(args) > 1 {
		if d, err := strconv.Atoi(args[1]); err == nil {
			searchDepth = d
		}
	}

	cwd, _ := os.Getwd()

	if target == ".." {
		saveHistory(cwd)
		fmt.Print(filepath.Dir(cwd))
		return
	}

	if strings.ToLower(target) == "back" {
		data, err := os.ReadFile(historyFile)
		if err == nil {
			prevDir := strings.TrimSpace(string(data))
			if _, err := os.Stat(prevDir); err == nil {
				saveHistory(cwd)
				fmt.Print(prevDir)
				return
			}
		}
		fmt.Fprintln(os.Stderr, lipgloss.NewStyle().Foreground(lipgloss.Color("11")).Render("No valid history found to go back to!"))
		return
	}

	var goodMatches []Match
	scoresDB := loadScores()

	if contentSearch {
		goodMatches = getContentMatches(cwd, target, searchDepth, allowedExts)
	} else {
		paths := getPaths(cwd, searchDepth, allowedExts)
		if len(paths) == 0 {
			fmt.Fprintf(os.Stderr, "No files or directories found up to depth %d.\n", searchDepth)
			return
		}

		var choices []string
		for k := range paths {
			choices = append(choices, k)
		}

		results := fuzzy.Find(target, choices)
		for _, r := range results {
			if r.Score > 0 {
				pathData := paths[r.Str]
				visits := scoresDB[pathData.AbsPath]
				bonus := visits * 2
				if bonus > 20 {
					bonus = 20
				}
				goodMatches = append(goodMatches, Match{
					RelPath: r.Str,
					AbsPath: pathData.AbsPath,
					Score:   r.Score + bonus,
					Visits:  visits,
					IsDir:   pathData.IsDir,
				})
			}
		}
	}

	sort.Slice(goodMatches, func(i, j int) bool {
		return goodMatches[i].Score > goodMatches[j].Score
	})

	if len(goodMatches) > 10 {
		goodMatches = goodMatches[:10]
	}

	if len(goodMatches) == 0 {
		fmt.Fprintf(os.Stderr, "No matches found for '%s'.\n", target)
		return
	}

	if len(goodMatches) == 1 || (len(goodMatches) > 1 && goodMatches[0].Score > goodMatches[1].Score+50) {
		bestMatch := goodMatches[0].AbsPath
		saveHistory(cwd)
		saveScore(bestMatch)
		fmt.Print(bestMatch)
		return
	}

	re := lipgloss.NewRenderer(os.Stderr)
	headerStyle := re.NewStyle().Foreground(lipgloss.Color("10")).Bold(true)
	cellStyle := re.NewStyle().Padding(0, 1)

	t := table.New().
		Border(lipgloss.NormalBorder()).
		BorderStyle(re.NewStyle().Foreground(lipgloss.Color("10"))).
		Headers("Opt", "Score", "Path Name", "Preview / Size").
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == 0 {
				return headerStyle.Padding(0, 1)
			}
			return cellStyle
		})

	// Define maximum widths for our columns to keep the terminal tidy
	maxPathLen := 40
	maxPreviewLen := 45

	for i, match := range goodMatches {
		scoreDisplay := fmt.Sprintf("%d", match.Score)
		if match.Visits > 0 {
			scoreDisplay += " ⭐"
		}

		peekData := getPeekString(match.AbsPath, match.IsDir)
		icon := "📄"
		if match.IsDir {
			icon = "📁"
		}

		// Safely truncate the path and the preview
		rawPath := fmt.Sprintf("%s %s", icon, match.RelPath)
		cleanPath := truncate(rawPath, maxPathLen)
		cleanPeek := truncate(peekData, maxPreviewLen)

		t.Row(fmt.Sprintf("[%d]", i+1), scoreDisplay, cleanPath, cleanPeek)
	}

	fmt.Fprintf(os.Stderr, "\n✨ Matches for '%s' (Depth %d) ✨\n", target, searchDepth)
	fmt.Fprintln(os.Stderr, t.Render())

	fmt.Fprint(os.Stderr, "\nSelect path (or 0 to cancel): ")

	reader := bufio.NewReader(os.Stdin)
	choiceStr, _ := reader.ReadString('\n')
	choiceStr = strings.TrimSpace(choiceStr)

	if choiceStr == "" {
		choiceStr = "1"
	}

	choiceIdx, err := strconv.Atoi(choiceStr)
	if err != nil || choiceIdx == 0 || choiceIdx > len(goodMatches) {
		return
	}

	selectedPath := goodMatches[choiceIdx-1].AbsPath
	saveHistory(cwd)
	saveScore(selectedPath)

	fmt.Print(selectedPath)
}