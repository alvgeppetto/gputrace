-- Menu bar access works without Accessibility permission (only Automation needed)
tell application "System Events"
	tell process "Xcode"
		set frontmost to true
		delay 0.5

		-- Try various Export menu item names
		try
			click menu item "Export..." of menu "File" of menu bar 1
		on error
			try
				click menu item "Export" of menu "File" of menu bar 1
			on error
				try
					click menu item "Export Trace..." of menu "File" of menu bar 1
				on error
					try
						click menu item "Save As..." of menu "File" of menu bar 1
					on error
						error "Export menu item not found"
					end try
				end try
			end try
		end try

		-- Wait for save/export sheet
		delay 1.5
		repeat with i from 1 to 30
			try
				if exists sheet 1 of window 1 then
					exit repeat
				end if
			end try
			delay 0.5
		end repeat

		-- Check if sheet actually appeared
		try
			if not (exists sheet 1 of window 1) then
				error "Export sheet did not appear after 15 seconds"
			end if
		end try

		delay 0.5

		-- Use keyboard to navigate
		-- Command+Shift+G to open "Go To Folder"
		try
			keystroke "g" using {command down, shift down}
			delay 1.5
			
			-- Type the output directory path
			keystroke "{{OUTPUT_DIR}}"
			delay 0.5
			keystroke return
			delay 1.5
			
			-- Select all and type filename
			keystroke "a" using {command down}
			delay 0.5
			keystroke "{{OUTPUT_NAME}}"
			delay 0.5

			-- Make sure "Embed performance data" is checked
			-- This requires UI element querying, so we wrap in try
			try
				tell sheet 1 of window 1
					set embedCheckbox to checkbox "Embed performance data"
					if value of embedCheckbox is 0 then
						click embedCheckbox
					end if
				end tell
			end try
			
			-- Press Enter to save
			keystroke return
			delay 1.0
		on error e
			error "Keyboard interaction failed: " & e
		end try

		return "Export initiated successfully"
	end tell
end tell
