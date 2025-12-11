import React, { useState } from 'react';
import { Sidebar } from './components/Sidebar';
import { GraphCanvas } from './components/GraphCanvas';
import { Inspector } from './components/Inspector';
import { TimelineView } from './components/TimelineView';
import { CostView } from './components/CostView';
import { TraceItem, NodeType } from './types';
import { MOCK_TRACE_DATA, INITIAL_NODES } from './constants';
import { Command, Maximize2, MoreHorizontal, Play, Save, Activity, Clock, BarChart2 } from 'lucide-react';

// Helper to find an item by ID in the tree
const findTraceItem = (items: TraceItem[], id: string): TraceItem | null => {
  for (const item of items) {
    if (item.id === id) return item;
    if (item.children) {
      const found = findTraceItem(item.children, id);
      if (found) return found;
    }
  }
  return null;
};

// Helper to create a transient TraceItem from visual node data if not found in trace tree
const createVisualItem = (id: string): TraceItem | null => {
    const node = INITIAL_NODES.find(n => n.id === id);
    if (!node) return null;

    let type = NodeType.BUFFER; // Default assumption for orphan nodes in this specific mock
    if (node.type === 'buffer') type = NodeType.BUFFER;

    return {
        id: node.id,
        label: node.data.label || id,
        type: type,
        description: 'Visual Node',
        properties: {
             'Label': node.data.label,
             'Type': node.data.type,
             'Size': node.data.size,
             'Source': 'Graph Node'
        }
    }
}

type ViewMode = 'overview' | 'timeline' | 'cost';

export default function App() {
  const [selectedItem, setSelectedItem] = useState<TraceItem | null>(null);
  const [selectedGraphId, setSelectedGraphId] = useState<string | null>(null);
  const [viewMode, setViewMode] = useState<ViewMode>('overview');

  const handleSelect = (id: string | null) => {
    setSelectedGraphId(id);

    if (!id) {
      setSelectedItem(null);
      return;
    }

    // Try to find in trace data first (e.g. encoders, dispatches)
    let item = findTraceItem(MOCK_TRACE_DATA, id);

    // If not found in trace hierarchy, check if it is a visual-only node (e.g. a buffer node)
    if (!item) {
        item = createVisualItem(id);
    }

    if (item) {
      setSelectedItem(item);
    } else {
        // If we selected something that isn't mapped, clear the inspector but keep the graph highlight
        setSelectedItem(null);
    }
  };

  return (
    <div className="flex flex-col h-screen w-screen bg-[#000] text-gray-200 font-sans overflow-hidden">

      {/* Top Application Bar (Simulating macOS App Bar) */}
      <div className="h-10 bg-[#2d2d2d] border-b border-black flex items-center px-4 justify-between select-none">
        <div className="flex items-center gap-4">
             <div className="flex gap-2 group">
                 <div className="w-3 h-3 rounded-full bg-red-500 group-hover:bg-red-600 transition-colors" />
                 <div className="w-3 h-3 rounded-full bg-yellow-500 group-hover:bg-yellow-600 transition-colors" />
                 <div className="w-3 h-3 rounded-full bg-green-500 group-hover:bg-green-600 transition-colors" />
             </div>
             <div className="h-6 w-[1px] bg-gray-700 mx-2" />
             <div className="flex gap-4 text-gray-400">
                <button className="hover:text-white transition-colors"><Command size={16} /></button>
                <button className="hover:text-white transition-colors"><Save size={16} /></button>
             </div>
        </div>

        {/* View Switcher / Title */}
        <div className="flex bg-[#1e1e1e] rounded-md p-0.5 border border-gray-700">
             <button
                className={`px-3 py-0.5 text-xs rounded-sm flex items-center gap-1.5 transition-colors ${viewMode === 'overview' ? 'bg-[#3b3b3b] text-white shadow-sm' : 'text-gray-400 hover:text-gray-200'}`}
                onClick={() => setViewMode('overview')}
             >
                <Activity size={12} /> Overview
             </button>
             <button
                className={`px-3 py-0.5 text-xs rounded-sm flex items-center gap-1.5 transition-colors ${viewMode === 'timeline' ? 'bg-[#3b3b3b] text-white shadow-sm' : 'text-gray-400 hover:text-gray-200'}`}
                onClick={() => setViewMode('timeline')}
             >
                <Clock size={12} /> Timeline
             </button>
             <button
                className={`px-3 py-0.5 text-xs rounded-sm flex items-center gap-1.5 transition-colors ${viewMode === 'cost' ? 'bg-[#3b3b3b] text-white shadow-sm' : 'text-gray-400 hover:text-gray-200'}`}
                onClick={() => setViewMode('cost')}
             >
                <BarChart2 size={12} /> Cost Graph
             </button>
        </div>

        <div className="flex items-center gap-3 text-xs text-gray-400">
            <span className="flex items-center gap-1 hover:text-white cursor-pointer px-2 py-1 hover:bg-gray-700 rounded transition-colors">
                <Play size={12} className="fill-current" /> Start Page
            </span>
            <MoreHorizontal size={16} />
        </div>
      </div>

      {/* Main Content Area */}
      <div className="flex-1 flex overflow-hidden">

        {/* Left Sidebar: Command List (Visible in Overview and Cost) */}
        {viewMode !== 'timeline' && (
            <Sidebar
                selectedId={selectedItem?.id || null}
                onSelect={(item) => handleSelect(item.id)}
            />
        )}

        {/* Center: Viewport */}
        <div className="flex-1 relative flex flex-col bg-[#111]">

            {viewMode === 'overview' && (
                <>
                    {/* Toolbar for Graph */}
                    <div className="h-8 bg-[#1e1e1e] border-b border-gray-800 flex items-center px-2 justify-between">
                        <div className="flex items-center gap-2">
                            <button className="p-1 hover:bg-gray-700 rounded text-gray-400 hover:text-white"><Maximize2 size={14} /></button>
                            <div className="h-4 w-[1px] bg-gray-700" />
                            <span className="text-[10px] text-gray-500 uppercase tracking-widest font-semibold ml-2">Dependency Graph</span>
                        </div>
                    </div>
                    <GraphCanvas
                        selectedId={selectedGraphId}
                        onSelect={handleSelect}
                    />
                </>
            )}

            {viewMode === 'timeline' && <TimelineView />}

            {viewMode === 'cost' && <CostView />}

        </div>

        {/* Right Sidebar: Inspector (Only in Overview) */}
        {viewMode === 'overview' && (
            <Inspector
                item={selectedItem}
                onSelect={handleSelect}
            />
        )}

      </div>

      {/* Status Bar */}
      <div className="h-6 bg-[#007acc] text-white flex items-center px-3 text-[10px] justify-between">
        <div className="flex gap-4">
            <span>Auto</span>
            <span>Ready</span>
        </div>
        <div>
             UTF-8
        </div>
      </div>
    </div>
  );
}
