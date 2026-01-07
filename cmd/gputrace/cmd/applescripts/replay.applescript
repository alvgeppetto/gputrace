-- Trigger replay in Xcode GPU trace window
tell application "Xcode"
	activate
	delay 0.5
end tell

tell application "System Events"
	tell process "Xcode"
		set frontmost to true
		delay 0.3

		set clicked to false
		
		-- 1. Try clicking "Replay" button directly
		try
			if exists button "Replay" of window 1 then
				click button "Replay" of window 1
				set clicked to true
				return "Clicked Replay button"
			end if
		end try
		
		-- 2. Try inside splitter groups
		if not clicked then
			try
				if exists button "Replay" of splitter group 1 of window 1 then
					click button "Replay" of splitter group 1 of window 1
					set clicked to true
					return "Clicked Replay in splitter"
				end if
			end try
		end if

		-- 3. Fallback to Ctrl+R keyboard shortcut
		if not clicked then
			-- Use Ctrl+R keyboard shortcut (works for GPU trace replay)
			keystroke "r" using {control down}
			delay 0.3
			return "Sent Ctrl+R to trigger replay"
		end if
	end tell
end tell
