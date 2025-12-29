-- Debug script to dump menu bar items (no Accessibility needed)
tell application "System Events"
	tell process "Xcode"
		set output to "=== Xcode Menu Bar Items ===" & return

		-- Dump all menus and their items
		try
			repeat with m in every menu of menu bar 1
				set menuName to name of m
				set output to output & return & "Menu: " & menuName & return
				try
					repeat with mi in every menu item of m
						set miName to name of mi
						-- Show all items that might be relevant
						if miName is not missing value and miName is not "" then
							set output to output & "  - " & miName & return
						end if
					end repeat
				on error errMsg
					set output to output & "  [Error reading items: " & errMsg & "]" & return
				end try
			end repeat
		on error errMsg
			set output to output & "Error accessing menus: " & errMsg & return
		end try

		return output
	end tell
end tell
