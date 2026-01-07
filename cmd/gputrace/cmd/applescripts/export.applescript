-- Export GPU trace with performance data
tell application "Xcode"
	activate
	delay 0.5
end tell

tell application "System Events"
	tell process "Xcode"
		set frontmost to true
		delay 0.5

		-- Retry loop for clicking Export menu
		set exportClicked to false
		repeat with i from 1 to 5
			if exportClicked then exit repeat
			
			-- Method 1: File > Export...
			try
				if exists menu item "Export…" of menu 1 of menu bar item "File" of menu bar 1 then
					click menu item "Export…" of menu 1 of menu bar item "File" of menu bar 1
					set exportClicked to true
				else if exists menu item "Export..." of menu 1 of menu bar item "File" of menu bar 1 then
					click menu item "Export..." of menu 1 of menu bar item "File" of menu bar 1
					set exportClicked to true
				end if
			end try
			
			-- Method 2: Keyboard shortcut
			if not exportClicked then
				try
					keystroke "e" using {command down, shift down}
					set exportClicked to true
				end try
			end if
			
			if not exportClicked then delay 1
		end repeat

		if not exportClicked then
			error "Could not interact with Export menu item after 5 attempts"
		end if

		-- Wait for export sheet to appear (increased timeout and checks)
		set sheetFound to false
		repeat with i from 1 to 30 -- Wait up to 15 seconds
			try
				if exists sheet 1 of window 1 then
					set sheetFound to true
					exit repeat
				end if
			end try
			delay 0.5
		end repeat

		if not sheetFound then
			error "Export sheet did not appear after clicking Export"
		end if

		delay 1.0

		-- Navigate to output directory
		try
			keystroke "g" using {command down, shift down}
			delay 1.5
			keystroke "{{OUTPUT_DIR}}"
			delay 0.5
			keystroke return
			delay 1.5
		on error
			-- Fallback if Go To Folder fails? Unlikely but let's log
		end try

		-- Set filename
		try
			keystroke "a" using {command down}
			delay 0.3
			keystroke "{{OUTPUT_NAME}}"
			delay 0.5
		end try

		-- Handle "Embed performance data" checkbox if present
		try
			tell sheet 1 of window 1
				set checkboxes to every checkbox
				repeat with cb in checkboxes
					try
						if name of cb contains "performance" or name of cb contains "Embed" then
							if value of cb is 0 then click cb
						end if
					end try
				end repeat
			end tell
		end try

		delay 0.5

		-- Click Save
		try
			click button "Save" of sheet 1 of window 1
		on error
			keystroke return
		end try

		delay 2
		return "Export initiated"
	end tell
end tell
