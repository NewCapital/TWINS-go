import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { useStore } from '@/store/useStore';
import { useShallow } from 'zustand/react/shallow';
import { ExecuteRPCCommand, GetRPCCommandList, GetRPCCommandCategories, GetRPCCommandHelp, GetRPCCommandDescriptions, GetRPCCategoryOrder } from '@wailsjs/go/main/App';
import { sanitizeText } from '@/shared/utils/sanitize';
import type { RPCCommandResult, ConsoleMessage } from '@/shared/types/tools.types';

interface CategorizedSuggestion {
  type: 'category' | 'command';
  text: string;
  category?: string;
}

export const ConsoleTab: React.FC = () => {
  const {
    consoleOutput,
    consoleHistory,
    rpcCommands,
    rpcCommandCategories,
    rpcCommandDescriptions,
    rpcCategoryOrder,
    addConsoleMessage,
    clearConsole,
    addToHistory,
    setRPCCommands,
    setRPCCommandCategories,
    setRPCCommandDescriptions,
    setRPCCategoryOrder,
  } = useStore(useShallow((s) => ({
    consoleOutput: s.consoleOutput,
    consoleHistory: s.consoleHistory,
    rpcCommands: s.rpcCommands,
    rpcCommandCategories: s.rpcCommandCategories,
    rpcCommandDescriptions: s.rpcCommandDescriptions,
    rpcCategoryOrder: s.rpcCategoryOrder,
    addConsoleMessage: s.addConsoleMessage,
    clearConsole: s.clearConsole,
    addToHistory: s.addToHistory,
    setRPCCommands: s.setRPCCommands,
    setRPCCommandCategories: s.setRPCCommandCategories,
    setRPCCommandDescriptions: s.setRPCCommandDescriptions,
    setRPCCategoryOrder: s.setRPCCategoryOrder,
  })));

  const [input, setInput] = useState('');
  const [historyIndex, setHistoryIndex] = useState(-1);
  const [suggestions, setSuggestions] = useState<CategorizedSuggestion[]>([]);
  const [showSuggestions, setShowSuggestions] = useState(false);
  const [selectedSuggestion, setSelectedSuggestion] = useState(0);
  const outputRef = useRef<HTMLDivElement>(null);
  const inputRef = useRef<HTMLInputElement>(null);
  const suggestionsRef = useRef<HTMLDivElement>(null);

  // Load RPC command list, categories, descriptions, and category order on mount.
  // Static fallback in the backend guarantees data is always available, so no retry needed.
  useEffect(() => {
    let mounted = true;

    Promise.all([
      GetRPCCommandList().catch(() => [] as string[]),
      GetRPCCommandCategories().catch(() => ({} as Record<string, string[]>)),
      GetRPCCommandDescriptions().catch(() => ({} as Record<string, string>)),
      GetRPCCategoryOrder().catch(() => [] as string[]),
    ]).then(([cmds, cats, descs, order]) => {
      if (!mounted) return;
      if (cmds && cmds.length > 0) setRPCCommands(cmds as string[]);
      if (cats && Object.keys(cats).length > 0) setRPCCommandCategories(cats as Record<string, string[]>);
      if (descs && Object.keys(descs).length > 0) setRPCCommandDescriptions(descs as Record<string, string>);
      if (order && order.length > 0) setRPCCategoryOrder(order as string[]);
    });

    return () => { mounted = false; };
  }, [setRPCCommands, setRPCCommandCategories, setRPCCommandDescriptions, setRPCCategoryOrder]);

  // Add welcome message with safety warning on first render
  const welcomeShownRef = useRef(false);
  useEffect(() => {
    if (!welcomeShownRef.current) {
      welcomeShownRef.current = true;
      interface NavigatorWithUAData extends Navigator { userAgentData?: { platform: string } }
      const isMac = ((navigator as NavigatorWithUAData).userAgentData?.platform ?? navigator.platform).toUpperCase().includes('MAC');
      const clearKey = isMac ? 'Cmd+L' : 'Ctrl+L';
      addConsoleMessage({
        type: 'info',
        text: `Welcome to the TWINS RPC console.\nUse up and down arrows to navigate history, and ${clearKey} to clear screen.\nType help for an overview of available commands.`,
        time: new Date().toLocaleTimeString('en-US', { hour12: false }),
      });
      addConsoleMessage({
        type: 'warning',
        text: 'WARNING: Scammers have been active, telling users to type commands here, stealing their wallet contents. Do not use this console without fully understanding the ramifications of a command.',
        time: new Date().toLocaleTimeString('en-US', { hour12: false }),
      });
    }
  // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Auto-scroll to bottom
  useEffect(() => {
    if (outputRef.current) {
      outputRef.current.scrollTop = outputRef.current.scrollHeight;
    }
  }, [consoleOutput]);

  // Scroll selected suggestion into view
  useEffect(() => {
    if (showSuggestions && suggestionsRef.current) {
      const selected = suggestionsRef.current.querySelector('[data-selected="true"]');
      if (selected) {
        selected.scrollIntoView({ block: 'nearest' });
      }
    }
  }, [selectedSuggestion, showSuggestions]);

  // Word-wrap a help line so continuation lines indent under the description column.
  const wrapHelpLine = useCallback((cmdPart: string, desc: string, maxWidth: number): string => {
    const full = cmdPart + desc;
    if (full.length <= maxWidth) return full + '\n';

    const indent = ' '.repeat(cmdPart.length);
    const words = desc.split(' ');
    let result = cmdPart;
    let lineLen = cmdPart.length;

    for (const word of words) {
      if (lineLen + word.length + 1 > maxWidth && lineLen > cmdPart.length) {
        result += '\n' + indent;
        lineLen = indent.length;
      }
      if (lineLen > cmdPart.length) {
        result += ' ';
        lineLen += 1;
      }
      result += word;
      lineLen += word.length;
    }
    return result + '\n';
  }, []);

  // Build categorized help text for local `help` command.
  // Category order comes from the backend to avoid frontend/backend drift.
  const buildCategorizedHelp = useCallback((): string => {
    const cats = rpcCommandCategories;
    const descs = rpcCommandDescriptions;
    const order = rpcCategoryOrder.length > 0 ? rpcCategoryOrder : Object.keys(cats);
    if (!cats || Object.keys(cats).length === 0) {
      return 'Command categories not yet loaded. Try again in a moment.';
    }

    let text = '== Available RPC Commands ==\n';
    for (const category of order) {
      const commands = cats[category];
      if (!commands || commands.length === 0) continue;
      text += `\n--- ${category} ---\n`;
      for (const cmd of commands) {
        const desc = descs?.[cmd];
        if (desc) {
          const cmdPart = `  ${cmd.padEnd(30)} `;
          text += wrapHelpLine(cmdPart, desc, 90);
        } else {
          text += `  ${cmd}\n`;
        }
      }
    }
    text += "\nUse 'help <command>' for detailed help on a specific command.";
    return text;
  }, [rpcCommandCategories, rpcCommandDescriptions, rpcCategoryOrder, wrapHelpLine]);

  const executeCommand = useCallback(async (command: string) => {
    const trimmed = command.trim();
    if (!trimmed) return;

    const now = new Date().toLocaleTimeString('en-US', { hour12: false });

    // Add command to output
    addConsoleMessage({ type: 'command', text: trimmed, time: now });
    addToHistory(trimmed);

    // Handle local commands
    if (trimmed === 'clear') {
      clearConsole();
      return;
    }

    // Local `help` (no args) shows categorized output
    if (trimmed === 'help') {
      addConsoleMessage({ type: 'reply', text: buildCategorizedHelp(), time: now });
      return;
    }

    // Local `help <command>` fetches detailed help directly (works without RPC server)
    const helpMatch = trimmed.match(/^help\s+(\S+)$/i);
    if (helpMatch) {
      const helpCmd = helpMatch[1].toLowerCase();
      try {
        const helpText = await GetRPCCommandHelp(helpCmd);
        addConsoleMessage({ type: 'reply', text: sanitizeText(helpText), time: now });
      } catch {
        addConsoleMessage({ type: 'error', text: `Failed to get help for '${sanitizeText(helpCmd)}'`, time: now });
      }
      return;
    }

    try {
      const result = await ExecuteRPCCommand(trimmed) as RPCCommandResult;

      if (result.error) {
        addConsoleMessage({ type: 'error', text: sanitizeText(result.error), time: result.time });
      } else {
        const text = typeof result.result === 'string'
          ? result.result
          : JSON.stringify(result.result, null, 2);
        addConsoleMessage({ type: 'reply', text: sanitizeText(text), time: result.time });
      }
    } catch (err) {
      addConsoleMessage({
        type: 'error',
        text: err instanceof Error ? err.message : 'Unknown error',
        time: now,
      });
    }
  }, [addConsoleMessage, addToHistory, clearConsole, buildCategorizedHelp]);

  // Build categorized suggestion list from filter text
  const buildCategorizedSuggestions = useCallback((filter: string): CategorizedSuggestion[] => {
    const cats = rpcCommandCategories;
    const items: CategorizedSuggestion[] = [];

    // If no categories loaded, fall back to flat list
    if (!cats || Object.keys(cats).length === 0) {
      const matches = rpcCommands
        .filter((cmd) => cmd.startsWith(filter.toLowerCase()))
        .slice(0, 20);
      return matches.map((cmd) => ({ type: 'command' as const, text: cmd }));
    }

    const lowerFilter = filter.toLowerCase();
    const order = rpcCategoryOrder.length > 0 ? rpcCategoryOrder : Object.keys(cats);
    for (const category of order) {
      const commands = cats[category];
      if (!commands) continue;
      const matches = commands.filter((cmd) => cmd.startsWith(lowerFilter));
      if (matches.length === 0) continue;
      items.push({ type: 'category', text: category });
      for (const cmd of matches) {
        items.push({ type: 'command', text: cmd, category });
      }
    }
    return items;
  }, [rpcCommandCategories, rpcCommands, rpcCategoryOrder]);

  // Get selectable indices (commands only, skip category headers)
  const selectableIndices = useMemo(() => {
    return suggestions
      .map((s, i) => s.type === 'command' ? i : -1)
      .filter((i) => i >= 0);
  }, [suggestions]);

  const handleKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    // Ctrl+L / Cmd+L to clear
    if (e.key === 'l' && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      clearConsole();
      return;
    }

    // Tab for auto-completion
    if (e.key === 'Tab') {
      e.preventDefault();
      if (showSuggestions && selectableIndices.length > 0) {
        const actualIndex = selectableIndices[selectedSuggestion];
        if (actualIndex !== undefined && suggestions[actualIndex]) {
          setInput(suggestions[actualIndex].text);
        }
        setShowSuggestions(false);
      } else {
        const filter = input.trim();
        const items = buildCategorizedSuggestions(filter);
        const commandItems = items.filter((s) => s.type === 'command');
        if (commandItems.length === 1) {
          setInput(commandItems[0].text);
        } else if (items.length > 0) {
          setSuggestions(items);
          setShowSuggestions(true);
          setSelectedSuggestion(0);
        }
      }
      return;
    }

    // Navigate suggestions
    if (showSuggestions && selectableIndices.length > 0) {
      if (e.key === 'ArrowDown') {
        e.preventDefault();
        setSelectedSuggestion((prev) => Math.min(prev + 1, selectableIndices.length - 1));
        return;
      }
      if (e.key === 'ArrowUp') {
        e.preventDefault();
        setSelectedSuggestion((prev) => Math.max(prev - 1, 0));
        return;
      }
      if (e.key === 'Escape') {
        setShowSuggestions(false);
        return;
      }
    }

    // Command history navigation
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      if (consoleHistory.length === 0) return;
      const newIndex = historyIndex === -1 ? consoleHistory.length - 1 : Math.max(0, historyIndex - 1);
      setHistoryIndex(newIndex);
      setInput(consoleHistory[newIndex]);
      return;
    }

    if (e.key === 'ArrowDown') {
      e.preventDefault();
      if (historyIndex === -1) return;
      const newIndex = historyIndex + 1;
      if (newIndex >= consoleHistory.length) {
        setHistoryIndex(-1);
        setInput('');
      } else {
        setHistoryIndex(newIndex);
        setInput(consoleHistory[newIndex]);
      }
      return;
    }

    // Execute on Enter
    if (e.key === 'Enter') {
      e.preventDefault();
      // If suggestions are shown and a command is selected, fill it instead of executing
      if (showSuggestions && selectableIndices.length > 0) {
        const actualIndex = selectableIndices[selectedSuggestion];
        if (actualIndex !== undefined && suggestions[actualIndex]) {
          setInput(suggestions[actualIndex].text);
        }
        setShowSuggestions(false);
        return;
      }
      setHistoryIndex(-1);
      executeCommand(input);
      setInput('');
      return;
    }

    // Hide suggestions on Escape only; other keys are handled by onChange filtering below
  };

  // Live-filter suggestions as user types
  const handleInputChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const newValue = e.target.value;
    setInput(newValue);

    if (showSuggestions) {
      const filter = newValue.trim();
      if (!filter) {
        setShowSuggestions(false);
      } else {
        const items = buildCategorizedSuggestions(filter);
        if (items.filter((s) => s.type === 'command').length > 0) {
          setSuggestions(items);
          setSelectedSuggestion(0);
        } else {
          setShowSuggestions(false);
        }
      }
    }
  };

  const getMessageColor = (type: ConsoleMessage['type']): string => {
    switch (type) {
      case 'command': return '#ffffff';
      case 'reply': return '#00ff00';
      case 'error': return '#ff7f7f';
      case 'warning': return '#ff7f7f';
      case 'info': return '#4a9eff';
      default: return '#ffffff';
    }
  };

  const getMessagePrefix = (type: ConsoleMessage['type']): string => {
    switch (type) {
      case 'command': return '> ';
      default: return '';
    }
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Output area */}
      <div
        ref={outputRef}
        style={{
          flex: 1,
          overflowY: 'auto',
          padding: '12px',
          backgroundColor: '#1a1a2e',
          fontFamily: 'monospace',
          fontSize: '13px',
          lineHeight: '1.5',
        }}
      >
        {consoleOutput.map((msg, i) => (
          <div key={i} style={{ color: getMessageColor(msg.type), whiteSpace: 'pre-wrap', wordBreak: 'break-all' }}>
            <span style={{ color: '#666', marginRight: '8px' }}>{msg.time}</span>
            <span>{getMessagePrefix(msg.type)}{msg.text}</span>
          </div>
        ))}
      </div>

      {/* Input area */}
      <div style={{ position: 'relative', borderTop: '1px solid #444' }}>
        {/* Suggestions popup */}
        {showSuggestions && suggestions.length > 0 && (
          <div
            ref={suggestionsRef}
            style={{
              position: 'absolute',
              bottom: '100%',
              left: 0,
              right: 0,
              backgroundColor: '#2a2a3e',
              border: '1px solid #555',
              maxHeight: '280px',
              overflowY: 'auto',
              zIndex: 10,
            }}
          >
            {suggestions.map((item, i) => {
              if (item.type === 'category') {
                return (
                  <div
                    key={`cat-${i}-${item.text}`}
                    style={{
                      padding: '4px 12px 2px',
                      color: '#888',
                      fontSize: '10px',
                      fontFamily: 'monospace',
                      textTransform: 'uppercase',
                      letterSpacing: '1px',
                      borderTop: i > 0 ? '1px solid #3a3a4e' : 'none',
                      marginTop: i > 0 ? '2px' : 0,
                      userSelect: 'none',
                    }}
                  >
                    {item.text}
                  </div>
                );
              }

              const selectableIdx = selectableIndices.indexOf(i);
              const isSelected = selectableIdx === selectedSuggestion;
              return (
                <div
                  key={`cmd-${i}-${item.text}`}
                  data-selected={isSelected}
                  onClick={() => {
                    setInput(item.text);
                    setShowSuggestions(false);
                    inputRef.current?.focus();
                  }}
                  style={{
                    padding: '3px 12px 3px 20px',
                    cursor: 'pointer',
                    backgroundColor: isSelected ? '#4a9eff' : 'transparent',
                    color: isSelected ? '#fff' : '#ccc',
                    fontFamily: 'monospace',
                    fontSize: '13px',
                  }}
                >
                  {item.text}
                </div>
              );
            })}
          </div>
        )}

        <div style={{ display: 'flex', alignItems: 'center', backgroundColor: '#1a1a2e' }}>
          <span style={{
            padding: '8px 4px 8px 12px',
            color: '#4a9eff',
            fontFamily: 'monospace',
            fontSize: '13px',
            userSelect: 'none',
          }}>
            &gt;
          </span>
          <input
            ref={inputRef}
            type="text"
            value={input}
            onChange={handleInputChange}
            onKeyDown={handleKeyDown}
            placeholder="Enter RPC command..."
            autoFocus
            autoComplete="off"
            spellCheck={false}
            title=""
            style={{
              flex: 1,
              padding: '8px',
              backgroundColor: 'transparent',
              border: 'none',
              outline: 'none',
              color: '#fff',
              fontFamily: 'monospace',
              fontSize: '13px',
            }}
          />
        </div>
      </div>
    </div>
  );
};
