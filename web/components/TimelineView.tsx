import React from 'react';
import { TIMELINE_TRACKS, TIMELINE_EVENTS } from '../constants';
import { ChevronDown, Filter } from 'lucide-react';

interface TimelineViewProps {
    selectedId: string | null;
    onSelect: (id: string | null) => void;
}

export const TimelineView: React.FC<TimelineViewProps> = ({ selectedId, onSelect }) => {
    // Total duration of the view (mocked as 1.5ms)
    const TOTAL_DURATION = 1.5;

    const [windowStart, setWindowStart] = React.useState(0);
    const [windowDuration, setWindowDuration] = React.useState(TOTAL_DURATION);
    const [scrubberPos, setScrubberPos] = React.useState<number | null>(null);
    const containerRef = React.useRef<HTMLDivElement>(null);

    const getLeft = (start: number) => {
        const relativeStart = start - windowStart;
        return `${(relativeStart / windowDuration) * 100}%`;
    };

    const getWidth = (duration: number) => `${(duration / windowDuration) * 100}%`;

    const handleWheel = (e: React.WheelEvent) => {
        // e.preventDefault(); // React synthetic events can't be prevented this way for wheel sometimes, but let's try

        if (e.shiftKey) {
            // Pan
            const panAmount = (e.deltaY / 1000) * windowDuration;
            setWindowStart(prev => Math.max(0, Math.min(TOTAL_DURATION - windowDuration, prev + panAmount)));
        } else if (e.ctrlKey || e.metaKey) {
            // Zoom
            const zoomFactor = 1 + (e.deltaY / 1000);
            setWindowDuration(prev => Math.max(0.1, Math.min(TOTAL_DURATION, prev * zoomFactor)));
        }
    };

    const handleMouseMove = (e: React.MouseEvent) => {
        if (!containerRef.current) return;
        const rect = containerRef.current.getBoundingClientRect();
        const x = e.clientX - rect.left;
        setScrubberPos(x);
    };

    return (
        <div className="flex-1 flex flex-col bg-[#111] overflow-hidden" onWheel={handleWheel}>
            {/* Timeline Toolbar */}
            <div className="h-8 bg-[#1e1e1e] border-b border-gray-800 flex items-center px-4 text-xs text-gray-400 gap-4">
                <div className="flex items-center gap-2 bg-[#111] border border-gray-700 rounded px-2 py-0.5">
                    <Filter size={10} />
                    <span className="text-[10px]">Filter Tracks</span>
                </div>
                <div className="flex-1 text-center font-mono text-[10px] text-gray-500">
                    Window: {windowDuration.toFixed(3)}ms (Start: {windowStart.toFixed(3)}ms)
                </div>
            </div>

            {/* Timeline Header (Ruler) */}
            <div className="h-6 bg-[#1a1a1a] border-b border-gray-800 flex">
                <div className="w-48 flex-shrink-0 border-r border-gray-800 bg-[#252526] flex items-center px-2 text-[10px] text-gray-400 font-bold">
                    Encoders
                </div>
                <div className="flex-1 relative overflow-hidden">
                    <div className="absolute inset-0 flex items-end">
                        {[0, 0.25, 0.5, 0.75, 1.0, 1.25, 1.5].map((tick) => {
                            if (tick < windowStart || tick > windowStart + windowDuration) return null;
                            const left = ((tick - windowStart) / windowDuration) * 100;
                            return (
                                <div key={tick} className="absolute h-full border-l border-gray-800/50" style={{ left: `${left}%` }}>
                                    <span className="absolute bottom-1 left-1 text-[9px] font-mono text-gray-500">{tick.toFixed(3)}</span>
                                </div>
                            );
                        })}
                    </div>
                </div>
            </div>

            {/* Tracks */}
            <div
                className="flex-1 overflow-y-auto custom-scrollbar relative"
                ref={containerRef}
                onMouseMove={handleMouseMove}
                onMouseLeave={() => setScrubberPos(null)}
            >
                {/* Scrubber Line */}
                {scrubberPos !== null && containerRef.current && (
                    <>
                        <div className="absolute top-0 bottom-0 w-px bg-blue-500 pointer-events-none z-50"
                            style={{ left: scrubberPos }} />
                        <div className="absolute top-0 text-[9px] bg-blue-900 text-blue-200 px-1 rounded z-50 pointer-events-none"
                            style={{ left: scrubberPos + 4 }}>
                            {((scrubberPos / containerRef.current.clientWidth) * windowDuration + windowStart).toFixed(3)}ms
                        </div>
                    </>
                )}

                {TIMELINE_TRACKS.map((track) => {
                    const trackEvents = TIMELINE_EVENTS.filter(e => e.trackId === track.id);
                    return (
                        <div key={track.id} className="flex border-b border-gray-800/50 h-8 hover:bg-[#1a1a1a] group">
                            {/* Track Label */}
                            <div className="w-48 flex-shrink-0 border-r border-gray-800 bg-[#1e1e1e] flex items-center px-2 text-[11px] text-gray-300 gap-2 relative z-10">
                                {track.type === 'encoder' ? <ChevronDown size={10} className="text-gray-500" /> : <div className="w-2.5" />}
                                {track.label}
                            </div>

                            {/* Track Lane */}
                            <div className="flex-1 relative bg-[#111] overflow-hidden">
                                {trackEvents.map((event) => {
                                    if (event.start + event.duration < windowStart || event.start > windowStart + windowDuration) return null;

                                    return (
                                        <div
                                            key={event.id}
                                            onClick={() => onSelect(event.id)}
                                            className={`absolute top-1 bottom-1 rounded-sm border text-[9px] text-white flex items-center px-1 overflow-hidden whitespace-nowrap cursor-pointer transition-all
                                            ${event.color} 
                                            ${selectedId === event.id ? 'ring-2 ring-white z-20 brightness-110' : 'border-black/20 hover:brightness-110'}
                                        `}
                                            style={{
                                                left: getLeft(event.start),
                                                width: getWidth(event.duration),
                                                minWidth: '2px'
                                            }}
                                            title={`${event.label} (${event.duration}ms)`}
                                        >
                                            <span className="truncate">{event.label}</span>
                                        </div>
                                    )
                                })}
                                {/* Grid lines background */}
                                <div className="absolute inset-0 pointer-events-none">
                                    {[0, 0.25, 0.5, 0.75, 1.0, 1.25, 1.5].map((tick) => {
                                        if (tick < windowStart || tick > windowStart + windowDuration) return null;
                                        const left = ((tick - windowStart) / windowDuration) * 100;
                                        return (
                                            <div key={tick} className="absolute h-full border-l border-gray-800/20" style={{ left: `${left}%` }} />
                                        );
                                    })}
                                </div>
                            </div>
                        </div>
                    );
                })}

                {/* Performance Counters Section (Bottom) */}
                <div className="mt-8">
                    <div className="bg-[#1e1e1e] border-y border-gray-700 px-2 py-1 text-[10px] font-bold text-gray-400 uppercase">
                        Counters
                    </div>
                    <div className="grid grid-cols-[150px_1fr] border-b border-gray-800 h-16">
                        <div className="flex items-center px-4 text-[10px] text-gray-400 bg-[#1a1a1a] border-r border-gray-800">
                            Active Cores
                        </div>
                        <div className="relative p-2">
                            {/* Mock graph line */}
                            <svg className="w-full h-full" preserveAspectRatio="none">
                                <path d="M0,40 L100,40 L200,30 L300,35 L400,20 L500,20 L600,25 L800,40" fill="none" stroke="#3b82f6" strokeWidth="2" vectorEffect="non-scaling-stroke" />
                            </svg>
                        </div>
                    </div>
                    <div className="grid grid-cols-[150px_1fr] border-b border-gray-800 h-16">
                        <div className="flex items-center px-4 text-[10px] text-gray-400 bg-[#1a1a1a] border-r border-gray-800">
                            Occupancy
                        </div>
                        <div className="relative p-2">
                            <svg className="w-full h-full" preserveAspectRatio="none">
                                <path d="M0,50 L800,50" fill="none" stroke="#4b5563" strokeWidth="1" strokeDasharray="4,4" vectorEffect="non-scaling-stroke" />
                                <path d="M0,45 L100,45 L200,45 L400,10 L500,10 L600,45 L800,45" fill="none" stroke="#eab308" strokeWidth="2" vectorEffect="non-scaling-stroke" />
                            </svg>
                        </div>
                    </div>
                </div>

            </div>
        </div>
    );
}
