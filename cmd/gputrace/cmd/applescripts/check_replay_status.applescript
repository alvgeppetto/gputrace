tell application "System Events"
	tell process "Xcode"
		-- Look for progress indicators
		try
			set allProgress to every progress indicator of window 1
			if (count of allProgress) > 0 then
				return "in_progress"
			end if
		end try

		-- Check for busy cursor or spinning indicators
		try
			repeat with sg in every splitter group of window 1
				set progList to every progress indicator of sg
				if (count of progList) > 0 then
					return "in_progress"
				end if
			end repeat
		end try

		-- No progress indicators found - assume complete
		return "complete"
	end tell
end tell
