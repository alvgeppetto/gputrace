-- Check if replay/profiling is complete
-- Priority: Show Performance button > Export button > disabled Stop button
tell application "System Events"
	tell process "Xcode"
		try
			tell window 1
				set allElements to entire contents

				-- Check for completion indicators first (these mean profiling is done)
				repeat with elem in allElements
					try
						if class of elem is button then
							set btnName to name of elem
							-- Show Performance button means profiling completed successfully
							if btnName is "Show Performance" then
								return "complete"
							end if
						end if
					end try
				end repeat

				-- Check Export button (also indicates completion)
				repeat with elem in allElements
					try
						if class of elem is button then
							set btnName to name of elem
							if btnName is "Export" then
								if enabled of elem then
									return "complete"
								end if
							end if
						end if
					end try
				end repeat

				-- Check for disabled Stop button (means not actively running)
				repeat with elem in allElements
					try
						if class of elem is button then
							set btnName to name of elem
							if btnName is "Stop" or btnName is "Stop GPU workload" then
								if not (enabled of elem) then
									return "complete"
								end if
								-- Stop button is enabled, still running
								return "running"
							end if
						end if
					end try
				end repeat

				-- Check Replay button as fallback
				repeat with elem in allElements
					try
						if class of elem is button then
							set btnName to name of elem
							if btnName is "Replay" then
								if enabled of elem then
									return "complete"
								else
									return "initializing"
								end if
							end if
						end if
					end try
				end repeat
			end tell
		end try
		return "unknown"
	end tell
end tell
