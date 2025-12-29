-- Menu bar access works without Accessibility permission (only Automation needed)
-- Prioritize menu-based approaches
tell application "System Events"
	tell process "Xcode"
		set frontmost to true
		delay 0.5

		-- Strategy 1: Try Document menu - Replay for GPU traces
		try
			click menu item "Replay" of menu "Document" of menu bar 1
			return "Clicked Replay via Document menu"
		end try

		-- Strategy 2: Try Debug menu - sometimes contains Replay
		try
			click menu item "Replay" of menu "Debug" of menu bar 1
			return "Clicked Replay via Debug menu"
		end try

		-- Strategy 3: Try Product menu
		try
			click menu item "Replay" of menu "Product" of menu bar 1
			return "Clicked Replay via Product menu"
		end try

		-- Strategy 4: Try Editor menu
		try
			click menu item "Replay" of menu "Editor" of menu bar 1
			return "Clicked Replay via Editor menu"
		end try

		-- Strategy 5: Try Control+R keyboard shortcut
		try
			keystroke "r" using {control down}
			delay 0.5
			return "Sent Control+R keystroke"
		end try

		-- Strategy 6: Try Command+R (Run shortcut, might work for replay)
		try
			keystroke "r" using {command down}
			delay 0.5
			return "Sent Command+R keystroke"
		end try

		-- Strategy 7: Send Enter (Return) key - Often the default action in the Replay bar
		try
			key code 36 -- Return key
			delay 0.5
			return "Sent Return key"
		end try

		error "Replay action failed - please click manually"
	end tell
end tell
