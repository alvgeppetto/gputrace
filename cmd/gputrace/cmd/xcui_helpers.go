package cmd

import "fmt"

func debugCheckExportMenu(app uintptr) error {
	menuBar := findElement(app, func(el uintptr) bool {
		return axString(el, "AXRole") == "AXMenuBar"
	})
	if menuBar == 0 {
		return fmt.Errorf("menubar not found")
	}

	// Find File menu
	fileMenu := findElement(menuBar, func(el uintptr) bool {
		return axString(el, "AXTitle") == "File"
	})
	if fileMenu == 0 {
		return fmt.Errorf("File menu not found")
	}

	// Click File to populate children (often needed for dynamic menus)
	if err := axAction(fileMenu, "AXPress"); err != nil {
		fmt.Printf("    Debug: Failed to open File menu: %v\n", err)
	}

	// Find Export item
	exportItem := findElement(fileMenu, func(el uintptr) bool {
		t := axString(el, "AXTitle")
		return t == "Export..." || t == "Export…"
	})

	if exportItem == 0 {
		fmt.Printf("    Debug: Export item NOT found in File menu\n")
		// Dump all items
		children := axChildren(fileMenu)
		for _, child := range children {
			fmt.Printf("      - %s (Enabled: %v)\n", axString(child, "AXTitle"), IsElementEnabled(child))
			cfRelease(child)
		}
		return nil
	}

	fmt.Printf("    Debug: Export item found. Enabled: %v\n", IsElementEnabled(exportItem))
	return nil
}
