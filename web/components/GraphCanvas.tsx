import React, { useCallback, useMemo, useEffect, useRef, useState } from 'react';
import ReactFlow, {
    Background,
    Controls,
    MiniMap,
    Node,
    Edge,
    useNodesState,
    useEdgesState,
    Handle,
    Position,
    NodeProps,
    ConnectionMode,
    Viewport,
    useReactFlow,
    ReactFlowProvider
} from 'reactflow';
import { Cpu, Play, Grid3X3, ArrowRightLeft, Clock, Layers, ChevronDown, ZoomIn, ZoomOut, Maximize, Lock, Unlock } from 'lucide-react';
import { INITIAL_EDGES, INITIAL_NODES } from '../constants';

// --- Custom Nodes ---

const DispatchNode = ({ data, selected }: NodeProps) => {
    const isHovered = data.hovered;
    return (
        <div className={`flex flex-col rounded-md shadow-xl min-w-[150px] bg-[#1a1a1a] border transition-all overflow-hidden ${selected ? 'border-blue-500 ring-2 ring-blue-500/20 shadow-blue-900/20' :
            isHovered ? 'border-blue-400 ring-1 ring-blue-400/30' :
                'border-gray-700'
            }`}>
            <Handle type="target" position={Position.Top} className="!bg-gray-500 !w-3 !h-1.5 !rounded-b-sm !top-0" />

            {/* Header with Type */}
            <div className="flex items-center gap-2 px-2 py-1 bg-gray-800/50 border-b border-gray-700/50">
                <div className="w-1.5 h-1.5 rounded-full bg-blue-500 shadow-[0_0_8px_rgba(59,130,246,0.6)]" />
                <span className="text-[9px] text-blue-200 font-bold uppercase tracking-wider opacity-80">Dispatch</span>
                <div className="flex-1" />
                {data.duration && (
                    <div className="flex items-center gap-1 bg-gray-900/50 px-1.5 rounded">
                        <Clock size={8} className="text-gray-500" />
                        <span className="text-[9px] text-gray-400 font-mono">{data.duration}</span>
                    </div>
                )}
            </div>

            {/* Body */}
            <div className="p-2 flex flex-col gap-1.5">
                {/* Function Name */}
                <div className="flex items-center gap-1.5">
                    <Play size={10} className="text-gray-500 flex-shrink-0" />
                    <span className="text-[10px] text-gray-200 font-semibold font-mono truncate" title={data.function}>
                        {data.function || 'kernel_main'}
                    </span>
                </div>

                {/* Stats Grid */}
                {(data.threads || data.grid) && (
                    <div className="grid grid-cols-2 gap-1 mt-1">
                        {data.threads && (
                            <div className="bg-gray-900/80 rounded px-1.5 py-1 border border-gray-800 flex flex-col">
                                <span className="text-[7px] text-gray-500 uppercase leading-none mb-0.5">Threads</span>
                                <span className="text-[9px] text-gray-300 font-mono leading-none">{data.threads}</span>
                            </div>
                        )}
                        {data.grid && (
                            <div className="bg-gray-900/80 rounded px-1.5 py-1 border border-gray-800 flex flex-col">
                                <span className="text-[7px] text-gray-500 uppercase leading-none mb-0.5">Grid</span>
                                <span className="text-[9px] text-gray-300 font-mono leading-none">{data.grid}</span>
                            </div>
                        )}
                    </div>
                )}
            </div>

            <Handle type="source" position={Position.Bottom} className="!bg-blue-500 !w-3 !h-1.5 !rounded-t-sm !bottom-0" />
        </div>
    );
};

const BarrierNode = ({ data, selected }: NodeProps) => {
    const isHovered = data.hovered;
    return (
        <div className={`px-2 py-1 rounded-full bg-gray-900 border flex items-center gap-2 ${selected ? 'border-blue-400 shadow-[0_0_10px_rgba(59,130,246,0.2)]' :
            isHovered ? 'border-blue-300 shadow-[0_0_8px_rgba(59,130,246,0.1)]' :
                'border-gray-700'
            }`}>
            <Handle type="target" position={Position.Top} className="!bg-gray-500 !w-1 !h-1 !opacity-0" />
            <div className="p-1 rounded-full bg-red-900/30">
                <ArrowRightLeft size={10} className="text-red-400" />
            </div>
            <div className="flex flex-col pr-1">
                <span className="text-[9px] text-gray-300 font-bold uppercase tracking-wide">Barrier</span>
                {data.type && <span className="text-[7px] text-gray-500 uppercase">{data.type}</span>}
            </div>
            <Handle type="source" position={Position.Bottom} className="!bg-gray-500 !w-1 !h-1 !opacity-0" />
        </div>
    );
};

const BufferNode = ({ data, selected }: NodeProps) => {
    const isHovered = data.hovered;
    return (
        <div className={`flex flex-col bg-[#0f172a] rounded border w-[140px] overflow-hidden ${selected ? 'border-blue-500 ring-1 ring-blue-500' :
            isHovered ? 'border-blue-400 ring-1 ring-blue-400/20' :
                'border-slate-800'
            }`}>
            <div className="px-2 py-1 bg-slate-900/50 flex items-center justify-between border-b border-slate-800">
                <div className="flex items-center gap-1.5">
                    <Layers size={10} className="text-blue-500" />
                    <span className="text-[9px] text-blue-200 font-semibold uppercase">Buffer</span>
                </div>
                {data.type && <span className="text-[8px] text-slate-500 bg-slate-800 px-1 rounded">{data.type}</span>}
            </div>

            <div className="p-2">
                <div className="text-[9px] text-slate-300 font-mono truncate mb-1" title={data.label}>{data.label}</div>
                <div className="flex items-center justify-between">
                    <span className="text-[8px] text-slate-500">Size</span>
                    <span className="text-[9px] text-slate-400 font-mono bg-slate-900 px-1 rounded border border-slate-800">{data.size}</span>
                </div>
            </div>

            <Handle type="source" position={Position.Left} className="!bg-blue-500 !w-1 !h-2 !rounded-r-sm" />
            <Handle type="target" position={Position.Right} className="!bg-blue-500 !w-1 !h-2 !rounded-l-sm" />
        </div>
    )
}

const EncoderGroupNode = ({ data, selected, id }: NodeProps) => {
    const { label, collapsed, onToggle } = data;

    return (
        <div
            className={`w-full h-full relative group transition-all duration-300 ease-in-out`}
        >
            <div
                className={`absolute inset-x-0 top-0 flex items-center gap-1.5 px-2 py-1.5 rounded-t border backdrop-blur-sm transition-colors cursor-pointer z-10
                    ${selected ? 'bg-red-900/80 border-red-500/50' : 'bg-gray-900/90 border-red-900/30 hover:bg-gray-800'}
                    ${collapsed ? 'rounded-b border-b' : 'border-b-0'}
                `}
                onClick={(e) => {
                    e.stopPropagation();
                    onToggle && onToggle(id);
                }}
            >
                <div className={`transition-transform duration-300 ${collapsed ? '-rotate-90' : 'rotate-0'}`}>
                    <ChevronDown size={12} className="text-gray-400" />
                </div>
                <Cpu size={12} className="text-red-500" />
                <span className="text-[11px] text-gray-200 font-medium tracking-wide select-none truncate flex-1">{label}</span>

                {collapsed && (
                    <span className="text-[9px] text-gray-500 font-mono bg-black/30 px-1.5 rounded ml-2">
                        {data.duration || '~'}
                    </span>
                )}
            </div>

            {/* Background body for the group */}
            <div className={`w-full h-full rounded border border-red-900/20 bg-gray-900/20 absolute top-0 left-0 -z-10 transition-all duration-300 ${collapsed ? 'opacity-0' : 'opacity-100'}`} />
        </div>
    )
};


const nodeTypes = {
    dispatch: DispatchNode,
    barrier: BarrierNode,
    group: EncoderGroupNode,
    buffer: BufferNode
};

interface GraphCanvasProps {
    selectedId: string | null;
    onSelect: (id: string | null) => void;
    hoveredId?: string | null;
    onHover?: (id: string | null) => void;
    nodes: Node[];
    edges: Edge[];
}

const GraphCanvasInner: React.FC<GraphCanvasProps> = ({ selectedId, onSelect, hoveredId, onHover, nodes: initialNodes, edges: initialEdges }) => {
    // We maintain nodes in state, but layout is derived from collapsed state
    const [nodes, setNodes, onNodesChange] = useNodesState(initialNodes);
    const [edges, setEdges, onEdgesChange] = useEdgesState(initialEdges);

    // Sync props to state when they change (e.g. data load)
    useEffect(() => {
        setNodes(initialNodes);
        setEdges(initialEdges);
    }, [initialNodes, initialEdges, setNodes, setEdges]);

    const [collapsedIds, setCollapsedIds] = useState<Set<string>>(new Set());
    const lastZoomRef = useRef(1);

    // Keep track of current nodes/edges to avoid stale state in selection effect
    const edgesRef = useRef(edges);
    const nodesRef = useRef(nodes);

    useEffect(() => {
        edgesRef.current = edges;
        nodesRef.current = nodes;
    }, [edges, nodes]);

    // Handle Layout Calculations when collapsedIds change
    useEffect(() => {
        setNodes((currentNodes) => {
            const groups = currentNodes.filter(n => n.type === 'group');
            // Sort groups by their INITIAL Y position to determine stack order reliably.
            // We use the ID to lookup original position from INITIAL_NODES to be safe,
            // or just rely on current if they haven't been reordered.
            // Let's rely on INITIAL_NODES for order.
            const sortedGroupIds = INITIAL_NODES
                .filter(n => n.type === 'group')
                .sort((a, b) => a.position.y - b.position.y)
                .map(n => n.id);

            // Create a map for easy lookup of original heights
            const originalHeights = new Map(INITIAL_NODES.map(n => [n.id, n.style?.height || 300]));

            let currentY = 0;
            const GAP = 50;
            const COLLAPSED_HEIGHT = 36; // Height of just the header

            // Helper to find index in sorted groups
            const getGroupIndex = (id: string) => sortedGroupIds.indexOf(id);

            const newNodes = currentNodes.map(node => {
                const newNode = { ...node, data: { ...node.data }, style: { ...node.style } };

                if (node.type === 'group') {
                    const isCollapsed = collapsedIds.has(node.id);
                    const h = isCollapsed ? COLLAPSED_HEIGHT : Number(originalHeights.get(node.id));

                    newNode.style = { ...newNode.style, height: h };
                    newNode.data = {
                        ...newNode.data,
                        collapsed: isCollapsed,
                        onToggle: (id: string) => {
                            setCollapsedIds(prev => {
                                const next = new Set(prev);
                                if (next.has(id)) next.delete(id);
                                else next.add(id);
                                return next;
                            });
                        }
                    };

                    // We need to set Y position.
                    // To do this correctly for a stack, we need to know the Y of all previous groups in the stack.
                    // This map function runs in parallel, so we can't easily accumulate inside map without external state
                    // or multiple passes.
                    // Let's assume we can compute it here if we iterate sortedGroupIds.
                    // But we are iterating 'currentNodes'.

                    // We'll fix positions in a second pass or lookup.
                }

                // Visibility of children
                if (node.parentNode) {
                    const isParentCollapsed = collapsedIds.has(node.parentNode);
                    newNode.hidden = isParentCollapsed;
                }

                return newNode;
            });

            // Second pass for Group Positions
            // Recalculate stack Ys
            let stackY = 0;
            // Map of group ID to new Y
            const groupYPositions = new Map<string, number>();

            for (const gid of sortedGroupIds) {
                const isCollapsed = collapsedIds.has(gid);
                const h = isCollapsed ? COLLAPSED_HEIGHT : Number(originalHeights.get(gid));
                groupYPositions.set(gid, stackY);
                stackY += h + GAP;
            }

            // Apply positions
            return newNodes.map(n => {
                if (n.type === 'group' && groupYPositions.has(n.id)) {
                    return { ...n, position: { ...n.position, y: groupYPositions.get(n.id)! } };
                }
                return n;
            });
        });
    }, [collapsedIds, setNodes]);

    // Handle Hover State Updates
    useEffect(() => {
        setNodes((nds) =>
            nds.map((node) => {
                const isHovered = node.id === hoveredId;
                // Only update if changed to avoid expensive renders? ReactFlow might handle it.
                // But we need to update data to trigger re-render of custom node
                if (node.data.hovered !== isHovered) {
                    return {
                        ...node,
                        data: {
                            ...node.data,
                            hovered: isHovered
                        }
                    };
                }
                return node;
            })
        );
    }, [hoveredId, setNodes]);


    const [layoutLocked, setLayoutLocked] = useState(false);
    const [autoCollapseEnabled, setAutoCollapseEnabled] = useState(true);
    const { zoomIn, zoomOut, fitView } = useReactFlow();

    // ... (existing effects)

    // Handle Zoom-based Auto-Grouping
    const onMove = useCallback((_: any, viewport: Viewport) => {
        if (!autoCollapseEnabled) return;

        const newZoom = viewport.zoom;
        const THRESHOLD = 0.55; // Zoom level to switch modes

        const wasZoomedOut = lastZoomRef.current < THRESHOLD;
        const isZoomedOut = newZoom < THRESHOLD;

        if (wasZoomedOut !== isZoomedOut) {
            // Threshold crossed
            if (isZoomedOut) {
                // Collapse All
                const allGroupIds = INITIAL_NODES.filter(n => n.type === 'group').map(n => n.id);
                setCollapsedIds(new Set(allGroupIds));
            } else {
                // Expand All
                setCollapsedIds(new Set());
            }
        }

        lastZoomRef.current = newZoom;
    }, [autoCollapseEnabled]);

    // ... (existing selection effect)

    // ... (existing handlers)

    const onNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
        onSelect(node.id);
    }, [onSelect]);

    const onPaneClick = useCallback(() => {
        onSelect(null);
    }, [onSelect]);

    const onNodeMouseEnter = useCallback((_: React.MouseEvent, node: Node) => {
        onHover && onHover(node.id);
    }, [onHover]);

    const onNodeMouseLeave = useCallback(() => {
        onHover && onHover(null);
    }, [onHover]);

    return (
        <div className="flex-1 h-full w-full bg-[#111] relative">
            <ReactFlow
                nodes={nodes}
                edges={edges}
                onNodesChange={onNodesChange}
                onEdgesChange={onEdgesChange}
                onNodeClick={onNodeClick}
                onPaneClick={onPaneClick}
                onNodeMouseEnter={onNodeMouseEnter}
                onNodeMouseLeave={onNodeMouseLeave}
                onMove={onMove}
                nodeTypes={nodeTypes}
                fitView
                connectionMode={ConnectionMode.Loose}
                attributionPosition="bottom-right"
                minZoom={0.1}
                maxZoom={2}
                proOptions={{ hideAttribution: true }}
                nodesDraggable={!layoutLocked}
                nodesConnectable={!layoutLocked}
            >
                <Background color="#333" gap={20} size={1} />
                <Controls className="bg-gray-800 border-gray-700 text-gray-200" />
                <MiniMap
                    nodeColor={(n) => {
                        if (n.type === 'group') return '#333';
                        if (n.type === 'dispatch') return '#3b82f6';
                        return '#eee';
                    }}
                    className="bg-gray-900 border border-gray-700 rounded-lg overflow-hidden"
                    maskColor="rgba(0,0,0,0.6)"
                />
            </ReactFlow>

            {/* Custom Control Panel */}
            <div className="absolute top-4 right-4 flex flex-col gap-1 bg-gray-900/90 p-1 rounded-md border border-gray-800 backdrop-blur-sm z-50 shadow-xl">
                <button onClick={() => zoomIn()} className="p-1.5 hover:bg-gray-700 rounded" title="Zoom In">
                    <ZoomIn size={14} className="text-gray-300" />
                </button>
                <button onClick={() => zoomOut()} className="p-1.5 hover:bg-gray-700 rounded" title="Zoom Out">
                    <ZoomOut size={14} className="text-gray-300" />
                </button>
                <button onClick={() => fitView()} className="p-1.5 hover:bg-gray-700 rounded" title="Fit View">
                    <Maximize size={14} className="text-gray-300" />
                </button>
                <div className="h-px bg-gray-700 my-1" />
                <button
                    onClick={() => setAutoCollapseEnabled(!autoCollapseEnabled)}
                    className={`p-1.5 rounded ${autoCollapseEnabled ? 'bg-blue-900/50' : 'hover:bg-gray-700'}`}
                    title="Auto-collapse on zoom"
                >
                    <Layers size={14} className={autoCollapseEnabled ? 'text-blue-400' : 'text-gray-300'} />
                </button>
                <button
                    onClick={() => setLayoutLocked(!layoutLocked)}
                    className={`p-1.5 rounded ${layoutLocked ? 'bg-amber-900/50' : 'hover:bg-gray-700'}`}
                    title="Lock layout"
                >
                    {layoutLocked ? <Lock size={14} className="text-amber-400" /> : <Unlock size={14} className="text-gray-300" />}
                </button>
            </div>

            {/* Overlay Breadcrumbs/Path */}
            <div className="absolute top-2 left-2 flex items-center gap-2 text-xs text-gray-400 bg-gray-900/80 px-3 py-1.5 rounded-full border border-gray-800 backdrop-blur-sm z-50">
                <span>Dependencies</span>
                <span>/</span>
                <span className="text-gray-200">{selectedId ? selectedId : 'No Selection'}</span>
            </div>

            {/* Zoom Hint */}
            <div className="absolute bottom-4 left-1/2 -translate-x-1/2 text-[10px] text-gray-500 bg-black/50 px-2 py-1 rounded backdrop-blur-sm pointer-events-none">
                Scroll to zoom • Zoom out to collapse groups
            </div>
        </div>
    );
};

export const GraphCanvas: React.FC<GraphCanvasProps> = (props) => (
    <ReactFlowProvider>
        <GraphCanvasInner {...props} />
    </ReactFlowProvider>
);
