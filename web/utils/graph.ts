import { Node, Edge, MarkerType } from 'reactflow';
import { TraceItem, NodeType } from '../types';

interface GraphData {
    nodes: Node[];
    edges: Edge[];
}

export const transformTraceToGraph = (root: TraceItem): GraphData => {
    const nodes: Node[] = [];
    const edges: Edge[] = [];

    // Config for layout
    let currentY = 0;
    const X_OFFSET_LEVELS = {
        [NodeType.GROUP]: 250,
        [NodeType.ENCODER]: 250,
        [NodeType.DISPATCH]: 50,
        [NodeType.BARRIER]: 75,
        [NodeType.BUFFER]: 600
    };
    const NODE_WIDTH = 300;

    // Recursive traversal to build graph
    // We mainly want to visualize:
    // - Command Buffers / Groups (as containers)
    // - Encoders (as containers or nodes)
    // - Dispatches (as nodes inside encoders)

    // Flat list of items to process for layout
    // For now, simpler approach: vertical stack of Encoders/Groups

    const processNode = (item: TraceItem, parentId: string | null, depth: number) => {

        // Skip Root for graph visualization usually, or make it a container
        if (item.type === NodeType.ROOT) {
            if (item.children) {
                item.children.forEach(child => processNode(child, null, depth));
            }
            return;
        }

        // Determine ReactFlow Node Type
        let rfType = 'default';
        let style = {};
        let position = { x: 0, y: currentY };
        let isGroup = false;

        if (item.type === NodeType.GROUP || item.type === NodeType.ENCODER) {
            rfType = 'group';
            isGroup = true;
            // Preset styles for containers
            style = {
                width: NODE_WIDTH,
                height: 200, // Dynamic height calculation needed later?
                backgroundColor: 'rgba(30, 30, 30, 0.4)',
                borderColor: item.type === NodeType.ENCODER ? '#ef4444' : '#3b82f6',
                borderWidth: 1,
                borderRadius: 8
            };

            // Layout: Place groups vertically
            position.x = X_OFFSET_LEVELS[NodeType.GROUP];
            // If it's a child group (e.g. encoder inside CB), offset slightly or keeping aligned
            if (parentId) {
                // parenting logic in ReactFlow handles relative position
                position = { x: 20, y: 50 }; // Hardcoded relative padding for now
            } else {
                currentY += 250; // increment global Y for top-level groups
            }
        } else if (item.type === NodeType.DISPATCH) {
            rfType = 'dispatch';
            position = { x: 50, y: 60 + (parseInt(item.id.split('-').pop() || '0') * 100) % 300 }; // naive stacking
        } else if (item.type === NodeType.BARRIER) {
            rfType = 'barrier';
            position = { x: 75, y: 150 };
        } else if (item.type === NodeType.BUFFER) {
            rfType = 'buffer';
            position = { x: 600, y: currentY }; // Place buffers to the side
        }

        // Construct Node
        const node: Node = {
            id: item.id,
            type: rfType,
            data: {
                label: item.label,
                function: item.properties?.['Function'],
                duration: item.stats?.duration,
                threads: item.stats?.threads,
                grid: item.properties?.['Grid Size'],
                ...item.properties
            },
            position: position,
            style: style,
        };

        if (parentId) {
            node.parentNode = parentId;
            node.extent = 'parent';
        }

        nodes.push(node);

        // Process Children
        if (item.children) {
            // Adjust height if parent is group based on children count (naive)
            if (isGroup && item.children.length > 0) {
                const estimatedHeight = item.children.length * 120 + 80;
                if (node.style) {
                    node.style.height = estimatedHeight;
                }
                if (!parentId) {
                    currentY = currentY - 250 + estimatedHeight + 50; // Adjust global Y
                }
            }

            let childY = 60;
            item.children.forEach(child => {
                // If child is Dispatch, stack them inside parent
                if (child.type === NodeType.DISPATCH || child.type === NodeType.BARRIER) {
                    const childNode = {
                        ...child,
                        // Pass layout info down if needed, or set position here
                    };
                    // Hacky: We create the node here to set relative position
                    let childRfType = child.type === NodeType.DISPATCH ? 'dispatch' : 'barrier';
                    nodes.push({
                        id: child.id,
                        type: childRfType,
                        data: {
                            label: child.label,
                            function: child.properties?.['Function'],
                            duration: child.stats?.duration,
                            threads: child.stats?.threads,
                            grid: child.properties?.['Grid Size'],
                        },
                        position: { x: 50, y: childY },
                        parentNode: item.id,
                        extent: 'parent'
                    });

                    // Add edge from previous sibling if exists (linear flow)
                    // (logic omitted for brevity, can happen in separate pass)

                    childY += 120;
                } else {
                    processNode(child, item.id, depth + 1);
                }
            });
        }
    };

    processNode(root, null, 0);

    // Generate basic edges for flow (Dispatcher -> Dispatcher)
    // This is a naive linear connection for now
    nodes.filter(n => n.type === 'dispatch').forEach((node, i, allDispatches) => {
        if (i < allDispatches.length - 1) {
            const next = allDispatches[i + 1];
            // Only connect if share same parent
            if (node.parentNode === next.parentNode) {
                edges.push({
                    id: `e-${node.id}-${next.id}`,
                    source: node.id,
                    target: next.id,
                    animated: true,
                    style: { stroke: '#3b82f6', strokeWidth: 2 },
                    markerEnd: { type: MarkerType.ArrowClosed, color: '#3b82f6' }
                });
            }
        }
    });

    return { nodes, edges };
};
