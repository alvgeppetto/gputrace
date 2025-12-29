tell application "Xcode"
	activate

	-- Wait for window to appear using direct Xcode scripting (no Accessibility needed)
	set windowCount to 0
	repeat with i from 1 to 30
		try
			set windowCount to count of windows
			if windowCount > 0 then
				return "Xcode window ready (found " & windowCount & " windows)"
			end if
		end try
		delay 1
	end repeat

	error "Xcode window did not appear within 30 seconds (found " & windowCount & " windows)"
end tell
