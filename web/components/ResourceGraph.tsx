import React, { useState, useRef } from 'react';

interface MemoryDataPoint {
    timestamp: number;
    system: number;
    video: number;
    shared: number;
    encoderId?: string;
}

interface ResourceGraphProps {
    data: MemoryDataPoint[];
    onHover?: (encoderId: string | null) => void;
}

export const ResourceGraph: React.FC<ResourceGraphProps> = ({ data, onHover }) => {
    const svgRef = useRef<SVGSVGElement>(null);
    const [tooltip, setTooltip] = useState<{ x: number; y: number; point: MemoryDataPoint } | null>(null);

    // Compute stacked area paths
    const maxMem = Math.max(...data.map(d => d.system + d.video + d.shared));
    const width = 800;
    const height = 200;

    const getY = (value: number) => height - (value / (maxMem || 1)) * height;

    // Generate SVG path for stacked areas
    // Stack order: System (bottom), Video (middle), Shared (top)

    const totalPoints = data.map((d, i) => {
        const x = (i / (data.length - 1)) * width;
        const y = getY(d.system + d.video + d.shared);
        return `${x},${y}`;
    });
    const totalPath = `M0,${height} ${totalPoints.join(' ')} L${width},${height} Z`;

    const sysVidPoints = data.map((d, i) => {
        const x = (i / (data.length - 1)) * width;
        const y = getY(d.system + d.video);
        return `${x},${y}`;
    });
    const sysVidPath = `M0,${height} ${sysVidPoints.join(' ')} L${width},${height} Z`;

    const sysPoints = data.map((d, i) => {
        const x = (i / (data.length - 1)) * width;
        const y = getY(d.system);
        return `${x},${y}`;
    });
    const sysPath = `M0,${height} ${sysPoints.join(' ')} L${width},${height} Z`;


    const handleMouseMove = (e: React.MouseEvent) => {
        if (!svgRef.current) return;
        const rect = svgRef.current.getBoundingClientRect();
        const x = e.clientX - rect.left;
        const index = Math.min(data.length - 1, Math.max(0, Math.floor((x / rect.width) * data.length)));
        const point = data[index];

        setTooltip({
            x: x,
            y: Math.min(e.clientY - rect.top, height - 80), // Keep tooltip in bounds
            point
        });

        if (onHover) {
            onHover(point.encoderId || null);
        }
    };

    return (
        <div className="relative h-full w-full bg-[#111] p-4">
            <div className="text-xs text-gray-400 mb-2 font-semibold">Memory Usage</div>
            <svg
                ref={svgRef}
                width="100%"
                height="100%"
                viewBox={`0 0 ${width} ${height}`}
                className="bg-[#1a1a1a] rounded border border-gray-800 cursor-crosshair"
                preserveAspectRatio="none"
                onMouseMove={handleMouseMove}
                onMouseLeave={() => {
                    setTooltip(null);
                    onHover && onHover(null);
                }}
            >
                {/* Render from back (largest) to front (smallest) */}
                <path d={totalPath} fill="#a855f7" fillOpacity={0.3} /> {/* Shared (Purple) */}
                <path d={sysVidPath} fill="#eab308" fillOpacity={0.3} /> {/* Video (Yellow) */}
                <path d={sysPath} fill="#3b82f6" fillOpacity={0.3} />   {/* System (Blue) */}

                {tooltip && (
                    <line x1={(data.indexOf(tooltip.point) / (data.length - 1)) * width} y1={0} x2={(data.indexOf(tooltip.point) / (data.length - 1)) * width} y2={height} stroke="white" strokeDasharray="4,4" strokeOpacity={0.5} />
                )}
            </svg>

            {/* Legend */}
            <div className="flex gap-4 mt-2 justify-center">
                <div className="flex items-center gap-1.5">
                    <div className="w-2 h-2 rounded-full bg-blue-500/50" />
                    <span className="text-[10px] text-gray-400">System</span>
                </div>
                <div className="flex items-center gap-1.5">
                    <div className="w-2 h-2 rounded-full bg-yellow-500/50" />
                    <span className="text-[10px] text-gray-400">Video</span>
                </div>
                <div className="flex items-center gap-1.5">
                    <div className="w-2 h-2 rounded-full bg-purple-500/50" />
                    <span className="text-[10px] text-gray-400">Shared</span>
                </div>
            </div>

            {tooltip && (
                <div style={{ left: tooltip.x + 20, top: tooltip.y }} className="absolute bg-gray-900/90 border border-gray-700 p-2 rounded text-[10px] text-gray-200 shadow-xl pointer-events-none z-50 backdrop-blur-sm">
                    <div className="font-mono mb-1 text-gray-400">{tooltip.point.timestamp.toFixed(2)}ms</div>
                    <div className="flex items-center justify-between gap-4"><span className="text-blue-400">System:</span> <span className="font-mono">{tooltip.point.system.toFixed(1)} MB</span></div>
                    <div className="flex items-center justify-between gap-4"><span className="text-yellow-400">Video:</span> <span className="font-mono">{tooltip.point.video.toFixed(1)} MB</span></div>
                    <div className="flex items-center justify-between gap-4"><span className="text-purple-400">Shared:</span> <span className="font-mono">{tooltip.point.shared.toFixed(1)} MB</span></div>
                    <div className="border-t border-gray-700 mt-1 pt-1 flex items-center justify-between gap-4 font-bold"><span>Total:</span> <span className="font-mono">{(tooltip.point.system + tooltip.point.video + tooltip.point.shared).toFixed(1)} MB</span></div>
                </div>
            )}
        </div>
    );
};
