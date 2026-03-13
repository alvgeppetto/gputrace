package axuiautomation

import appleax "github.com/tmc/apple/x/axuiautomation"

type (
	Application      = appleax.Application
	Element          = appleax.Element
	ElementPredicate = appleax.ElementPredicate
	ElementQuery     = appleax.ElementQuery
	Error            = appleax.Error
	Observer         = appleax.Observer
	ObserverEvent    = appleax.ObserverEvent
	ObserverHandler  = appleax.ObserverHandler
	Point            = appleax.Point
	Rect             = appleax.Rect
	Size             = appleax.Size
	TraversalMode    = appleax.TraversalMode
)

const (
	BFS TraversalMode = appleax.BFS
	DFS TraversalMode = appleax.DFS
)

var (
	ErrAPIDisabled       = appleax.ErrAPIDisabled
	ErrActionUnsupported = appleax.ErrActionUnsupported
	ErrElementNotFound   = appleax.ErrElementNotFound
	ErrInvalidElement    = appleax.ErrInvalidElement
	ErrNotRunning        = appleax.ErrNotRunning
	ErrTimeout           = appleax.ErrTimeout

	NewApplication        = appleax.NewApplication
	NewApplicationFromPID = appleax.NewApplicationFromPID
	NewObserver           = appleax.NewObserver

	IsProcessTrusted         = appleax.IsProcessTrusted
	PromptForAccessibility   = appleax.PromptForAccessibility
	CheckAccessibilityAccess = appleax.CheckAccessibilityAccess

	SendEscape    = appleax.SendEscape
	SendReturn    = appleax.SendReturn
	SendKeyCombo  = appleax.SendKeyCombo
	SendCmdShiftG = appleax.SendCmdShiftG
)
