import React from 'react';
import { COST_DATA, MOCK_SOURCE_CODE } from '../constants';
import { Code, ListFilter } from 'lucide-react';

import { ResourceGraph } from './ResourceGraph';

// Mock Memory Data
const MOCK_MEMORY_DATA = Array.from({ length: 100 }, (_, i) => ({
    timestamp: i * 0.015,
    system: 200 + Math.sin(i * 0.1) * 50,
    video: 400 + Math.cos(i * 0.05) * 100 + (i > 40 && i < 60 ? 300 : 0), // Spike
    shared: 100 + Math.random() * 20,
    encoderId: i > 40 && i < 60 ? 'encoder_1' : undefined
}));

export const CostView: React.FC = () => {
    return (
        <div className="flex-1 flex flex-col bg-[#111] overflow-hidden">
            {/* Top Split: Graph */}
            <div className="flex-1 flex flex-col min-h-0 border-b border-gray-800">
                <div className="h-8 bg-[#1e1e1e] border-b border-gray-800 flex items-center px-2 text-xs text-gray-400 gap-2">
                    <span className="font-semibold text-gray-300">Cost & Resources</span>
                    <div className="flex-1" />
                    <ListFilter size={14} />
                </div>

                <div className="flex flex-1 overflow-hidden">
                    {/* List of Encoders/Kernels */}
                    <div className="w-64 bg-[#1a1a1a] border-r border-gray-800 overflow-y-auto custom-scrollbar flex flex-col">
                        <div className="flex text-[10px] text-gray-500 px-2 py-1 bg-[#252526] border-b border-gray-800">
                            <span className="flex-1">Encoders</span>
                            <span>Cost %</span>
                        </div>
                        {COST_DATA.map((item, idx) => (
                            <div key={idx} className={`flex items-center px-2 py-1 text-[11px] border-b border-gray-800/50 hover:bg-[#2d2d2d] cursor-pointer ${idx === 0 ? 'bg-[#2d2d2d]' : ''}`}>
                                <div className="flex-1 truncate mr-2 text-gray-300" title={item.name}>
                                    <span className="inline-block w-2 h-2 rounded-full mr-2" style={{ backgroundColor: item.color }}></span>
                                    {item.name}
                                </div>
                                <span className="text-gray-400 font-mono">{item.cost.toFixed(2)}%</span>
                            </div>
                        ))}
                    </div>

                    {/* Main Chart Area */}
                    <div className="flex-1 bg-[#111] flex flex-col overflow-hidden">
                        {/* Cost Bars */}
                        <div className="p-4 overflow-y-auto custom-scrollbar flex-shrink-0 max-h-[50%] border-b border-gray-800">
                            <div className="text-xs text-gray-400 mb-2 font-semibold">Kernel Costs</div>
                            <div className="flex flex-col gap-1">
                                {COST_DATA.map((item, idx) => (
                                    <div key={idx} className="flex flex-col mb-2">
                                        <div className="flex justify-between text-[10px] text-gray-400 mb-0.5">
                                            <span className="font-mono text-blue-400">{item.name}</span>
                                            <span>{item.cost}%</span>
                                        </div>
                                        <div className="h-4 w-full bg-[#1e1e1e] rounded-sm overflow-hidden relative">
                                            <div
                                                className="h-full absolute top-0 left-0"
                                                style={{ width: `${item.cost}%`, backgroundColor: item.color }}
                                            />
                                        </div>
                                    </div>
                                ))}
                            </div>
                        </div>

                        {/* Resource Graph */}
                        <div className="flex-1 min-h-0">
                            <ResourceGraph data={MOCK_MEMORY_DATA} />
                        </div>
                    </div>
                </div>
            </div>

            {/* Bottom Split: Source Code */}
            <div className="h-1/3 bg-[#1e1e1e] flex flex-col">
                <div className="h-8 bg-[#252526] border-b border-gray-800 flex items-center px-3 gap-2">
                    <Code size={14} className="text-gray-400" />
                    <span className="text-xs text-gray-300 font-medium">Source Files</span>
                    <span className="text-xs text-gray-500">kernel.metal</span>
                </div>
                <div className="flex-1 overflow-auto p-2 bg-[#1a1a1a]">
                    <pre className="font-mono text-[11px] text-gray-300 leading-relaxed">
                        <code dangerouslySetInnerHTML={{
                            __html: MOCK_SOURCE_CODE
                                .replace(/template/g, '<span class="text-purple-400">template</span>')
                                .replace(/typename/g, '<span class="text-blue-400">typename</span>')
                                .replace(/void/g, '<span class="text-red-400">void</span>')
                                .replace(/device/g, '<span class="text-orange-400">device</span>')
                                .replace(/const/g, '<span class="text-purple-400">const</span>')
                                .replace(/int/g, '<span class="text-yellow-400">int</span>')
                                .replace(/uint/g, '<span class="text-yellow-400">uint</span>')
                                .replace(/if/g, '<span class="text-purple-400">if</span>')
                                .replace(/for/g, '<span class="text-purple-400">for</span>')
                                .replace(/(\/\/.*)/g, '<span class="text-gray-500">$1</span>')
                        }} />
                    </pre>
                </div>
            </div>
        </div>
    );
}
