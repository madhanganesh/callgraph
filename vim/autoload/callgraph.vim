" autoload/callgraph.vim — Call graph popup for Go, Python, and Rust files.
"
" Requires: callgraph binary in $PATH (or set g:callgraph_binary).
" Works in Vim 8.2+ (popup) and Neovim 0.9+ (floating window).

" Parallel array: s:locations[i] corresponds to display line i+1.
" Each entry is {'file': ..., 'line': ...}.
let s:locations = []

" ─── Public ──────────────────────────────────────────────────────────────────

let s:supported_filetypes = {'go': 1, 'python': 1, 'rust': 1}

function! callgraph#show(...) abort
  let l:direction = a:0 >= 1 ? a:1 : 'callers'

  if !has_key(s:supported_filetypes, &filetype)
    echohl WarningMsg | echom 'callgraph: unsupported filetype (need go, python, or rust)' | echohl None
    return
  endif

  let l:bin  = get(g:, 'callgraph_binary', 'callgraph')
  let l:file = expand('%:p')
  let l:line = line('.')
  let l:col  = col('.')

  echom 'callgraph: loading...'

  let l:data = s:run_callgraph(l:bin, l:file, l:line, l:col, l:direction)
  if type(l:data) != v:t_dict
    return
  endif

  " Interface picker: real popup. Selecting an entry re-invokes the CLI
  " rooted at the chosen impl and renders the tree in place.
  let l:impls = get(l:data, 'implementations', [])
  if !empty(l:impls)
    call s:show_picker(l:bin, l:impls, l:direction)
    echo ''
    return
  endif

  let s:locations = []
  let l:lines = []
  let l:initial = 1

  if l:direction ==# 'callees'
    call s:write_callee_node(l:data, '', v:true, v:true, l:lines)
  else
    let l:paths = []
    call s:collect_paths(l:data, [], l:paths)
    let l:tree = s:merge_paths(l:paths)
    for l:i in range(len(l:tree))
      call s:write_tree_node(l:tree[l:i], '', l:i == len(l:tree) - 1, l:lines)
    endfor
    let l:initial = s:first_chain_leaf_index(l:tree) + 1
  endif

  if empty(l:lines)
    echom 'callgraph: no results'
    return
  endif

  if has('nvim')
    call s:show_float(l:lines, l:initial)
  else
    call s:show_popup(l:lines, l:initial)
  endif
  echo ''
endfunction

" Show a short LLM summary of the function under cursor in a popup.
" If the cursor resolves to an interface method with multiple real impls,
" first show a picker; on selection, summarize the picked impl.
function! callgraph#summarize() abort
  if !has_key(s:supported_filetypes, &filetype)
    echohl WarningMsg | echom 'callgraph: unsupported filetype (need go, python, or rust)' | echohl None
    return
  endif
  let l:bin = get(g:, 'callgraph_binary', 'callgraph')
  call s:summarize_at(l:bin, expand('%:p'), line('.'), col('.'))
endfunction

function! s:summarize_at(bin, file, line, col) abort
  let l:cmd = printf('%s summarize --file=%s --line=%d --col=%d',
        \ shellescape(a:bin), shellescape(a:file), a:line, a:col)
  if exists('g:callgraph_llm_cmd') && !empty(g:callgraph_llm_cmd)
    let l:cmd .= ' --llm-cmd=' . shellescape(g:callgraph_llm_cmd)
  endif

  echom 'callgraph: summarizing...'
  let l:raw = system(l:cmd)
  if v:shell_error != 0
    echohl ErrorMsg
    echom 'callgraph: ' . substitute(l:raw, '\n', ' ', 'g')
    echohl None
    return
  endif

  let l:data = json_decode(l:raw)
  if type(l:data) != v:t_dict
    echom 'callgraph: bad response'
    return
  endif

  let l:impls = get(l:data, 'implementations', [])
  if !empty(l:impls)
    call s:show_summary_picker(a:bin, l:impls)
    echo ''
    return
  endif

  let l:summary = get(l:data, 'summary', '')
  if empty(l:summary)
    echom 'callgraph: empty summary'
    return
  endif
  let l:lines = split(l:summary, '\n')
  if has('nvim')
    call s:show_text_float(l:lines, ' Summary ')
  else
    call s:show_text_popup(l:lines, ' Summary ')
  endif
  echo ''
endfunction

" Picker state for the summary flow — separate from the call-graph picker
" so each can be open without trampling the other.
let s:sum_picker_impls = []
let s:sum_picker_bin = ''

function! s:show_summary_picker(bin, impls) abort
  let s:sum_picker_impls = a:impls
  let s:sum_picker_bin = a:bin

  let l:lines = []
  for l:impl in a:impls
    call add(l:lines, printf('%s (%s)', l:impl.name, get(l:impl, 'pkg', '')))
  endfor

  if has('nvim')
    call s:sum_picker_float(l:lines)
  else
    call s:sum_picker_popup(l:lines)
  endif
endfunction

function! s:sum_picker_popup(lines) abort
  let l:winid = popup_atcursor(a:lines, #{
        \ border:     [],
        \ padding:    [0, 1, 0, 1],
        \ maxwidth:   80,
        \ maxheight:  20,
        \ filter:     function('s:sum_picker_popup_filter'),
        \ mapping:    0,
        \ cursorline: 1,
        \ title:      ' Pick implementation ',
        \ })
  call win_execute(l:winid, 'call cursor(1, 1)')
endfunction

function! s:sum_picker_popup_filter(winid, key) abort
  if a:key ==# 'q' || a:key ==# "\<Esc>"
    call popup_close(a:winid)
    return 1
  endif
  if a:key ==# 'j' || a:key ==# "\<Down>"
    call win_execute(a:winid, 'normal! j')
    return 1
  endif
  if a:key ==# 'k' || a:key ==# "\<Up>"
    call win_execute(a:winid, 'normal! k')
    return 1
  endif
  if a:key ==# "\<CR>"
    let l:idx = getcurpos(a:winid)[1] - 1
    call popup_close(a:winid)
    call s:sum_resolve_pick(l:idx)
    return 1
  endif
  return 0
endfunction

function! s:sum_picker_float(lines) abort
  let l:buf = nvim_create_buf(v:false, v:true)
  call nvim_buf_set_lines(l:buf, 0, -1, v:false, a:lines)
  call setbufvar(l:buf, '&modifiable', 0)
  call setbufvar(l:buf, '&bufhidden', 'wipe')
  call setbufvar(l:buf, '&buftype', 'nofile')

  let l:width  = min([max(map(copy(a:lines), 'strwidth(v:val)')) + 4, 80])
  let l:height = min([len(a:lines), 20])
  let l:opts = {
        \ 'relative': 'cursor', 'row': 1, 'col': 0,
        \ 'width': l:width, 'height': l:height,
        \ 'style': 'minimal', 'border': 'rounded',
        \ }
  if has('nvim-0.9')
    let l:opts.title     = ' Pick implementation '
    let l:opts.title_pos = 'center'
  endif
  call nvim_open_win(l:buf, v:true, l:opts)
  setlocal cursorline
  nnoremap <buffer> <silent> q     <Cmd>close<CR>
  nnoremap <buffer> <silent> <Esc> <Cmd>close<CR>
  nnoremap <buffer> <silent> <CR>  <Cmd>call <SID>sum_float_select()<CR>
endfunction

function! s:sum_float_select() abort
  let l:idx = line('.') - 1
  close
  call s:sum_resolve_pick(l:idx)
endfunction

function! s:sum_resolve_pick(idx) abort
  if a:idx < 0 || a:idx >= len(s:sum_picker_impls)
    return
  endif
  let l:pick = s:sum_picker_impls[a:idx]
  call s:summarize_at(s:sum_picker_bin, l:pick.file, l:pick.line, get(l:pick, 'col', 1))
endfunction

" Plain text popup (no selection logic) — used for summaries.
function! s:show_text_popup(lines, title) abort
  call popup_atcursor(a:lines, #{
        \ border:    [],
        \ padding:   [0, 1, 0, 1],
        \ maxwidth:  80,
        \ maxheight: 20,
        \ title:     a:title,
        \ wrap:      1,
        \ mapping:   0,
        \ filter:    function('s:text_popup_filter'),
        \ })
endfunction

function! s:text_popup_filter(winid, key) abort
  if a:key ==# 'q' || a:key ==# "\<Esc>"
    call popup_close(a:winid)
    return 1
  endif
  return 0
endfunction

function! s:show_text_float(lines, title) abort
  let l:buf = nvim_create_buf(v:false, v:true)
  call nvim_buf_set_lines(l:buf, 0, -1, v:false, a:lines)
  call setbufvar(l:buf, '&modifiable', 0)
  call setbufvar(l:buf, '&bufhidden', 'wipe')
  call setbufvar(l:buf, '&buftype', 'nofile')
  call setbufvar(l:buf, '&wrap', 1)

  let l:width  = min([max(map(copy(a:lines), 'strwidth(v:val)')) + 4, 80])
  let l:height = min([len(a:lines), 20])

  let l:opts = {
        \ 'relative': 'cursor',
        \ 'row':      1,
        \ 'col':      0,
        \ 'width':    l:width,
        \ 'height':   l:height,
        \ 'style':    'minimal',
        \ 'border':   'rounded',
        \ }
  if has('nvim-0.9')
    let l:opts.title     = a:title
    let l:opts.title_pos = 'center'
  endif

  call nvim_open_win(l:buf, v:true, l:opts)
  nnoremap <buffer> <silent> q     <Cmd>close<CR>
  nnoremap <buffer> <silent> <Esc> <Cmd>close<CR>
endfunction

" Run the CLI and return decoded JSON dict, or 0 on error.
function! s:run_callgraph(bin, file, line, col, direction) abort
  let l:cmd = printf('%s --file=%s --line=%d --col=%d --direction=%s --format=json',
        \ shellescape(a:bin),
        \ shellescape(a:file),
        \ a:line, a:col, a:direction)
  let l:raw = system(l:cmd)
  if v:shell_error != 0
    echohl ErrorMsg
    echom 'callgraph: ' . substitute(l:raw, '\n', ' ', 'g')
    echohl None
    return 0
  endif
  return json_decode(l:raw)
endfunction

" Picker state — set when a picker popup is open so the selection callback
" can re-invoke the CLI with the right context.
let s:picker_impls = []
let s:picker_bin = ''
let s:picker_direction = ''

" Show a popup of implementations. Selecting one (Enter) closes the picker
" and renders the call graph rooted at that impl.
function! s:show_picker(bin, impls, direction) abort
  let s:picker_impls = a:impls
  let s:picker_bin = a:bin
  let s:picker_direction = a:direction

  let l:lines = []
  for l:i in range(len(a:impls))
    let l:impl = a:impls[l:i]
    call add(l:lines, printf('%s (%s)', l:impl.name, get(l:impl, 'pkg', '')))
  endfor

  if has('nvim')
    call s:show_picker_float(l:lines)
  else
    call s:show_picker_popup(l:lines)
  endif
endfunction

function! s:show_picker_popup(lines) abort
  call popup_atcursor(a:lines, #{
        \ border:     [],
        \ padding:    [0, 1, 0, 1],
        \ maxwidth:   80,
        \ maxheight:  20,
        \ filter:     function('s:picker_popup_filter'),
        \ mapping:    0,
        \ cursorline: 1,
        \ title:      ' Pick implementation ',
        \ })
endfunction

function! s:picker_popup_filter(winid, key) abort
  if a:key ==# 'q' || a:key ==# "\<Esc>"
    call popup_close(a:winid)
    return 1
  endif
  if a:key ==# 'j' || a:key ==# "\<Down>"
    call win_execute(a:winid, 'normal! j')
    return 1
  endif
  if a:key ==# 'k' || a:key ==# "\<Up>"
    call win_execute(a:winid, 'normal! k')
    return 1
  endif
  if a:key ==# "\<CR>"
    let l:pos = getcurpos(a:winid)
    let l:idx = l:pos[1] - 1
    call popup_close(a:winid)
    call s:resolve_pick(l:idx)
    return 1
  endif
  return 0
endfunction

function! s:show_picker_float(lines) abort
  let l:buf = nvim_create_buf(v:false, v:true)
  call nvim_buf_set_lines(l:buf, 0, -1, v:false, a:lines)
  call setbufvar(l:buf, '&modifiable', 0)
  call setbufvar(l:buf, '&bufhidden', 'wipe')
  call setbufvar(l:buf, '&buftype', 'nofile')

  let l:width  = min([max(map(copy(a:lines), 'strwidth(v:val)')) + 4, 80])
  let l:height = min([len(a:lines), 20])
  let l:opts = {
        \ 'relative':  'cursor',
        \ 'row':       1,
        \ 'col':       0,
        \ 'width':     l:width,
        \ 'height':    l:height,
        \ 'style':     'minimal',
        \ 'border':    'rounded',
        \ }
  if has('nvim-0.9')
    let l:opts.title     = ' Pick implementation '
    let l:opts.title_pos = 'center'
  endif
  call nvim_open_win(l:buf, v:true, l:opts)
  setlocal cursorline

  nnoremap <buffer> <silent> q     <Cmd>close<CR>
  nnoremap <buffer> <silent> <Esc> <Cmd>close<CR>
  nnoremap <buffer> <silent> <CR>  <Cmd>call <SID>picker_float_select()<CR>
endfunction

function! s:picker_float_select() abort
  let l:idx = line('.') - 1
  close
  call s:resolve_pick(l:idx)
endfunction

" Re-invoke the CLI rooted at the picked impl and render its tree.
function! s:resolve_pick(idx) abort
  if a:idx < 0 || a:idx >= len(s:picker_impls)
    return
  endif
  let l:pick = s:picker_impls[a:idx]
  echom 'callgraph: loading...'
  let l:data = s:run_callgraph(s:picker_bin, l:pick.file, l:pick.line,
        \ get(l:pick, 'col', 1), s:picker_direction)
  if type(l:data) != v:t_dict
    return
  endif

  let s:locations = []
  let l:lines = []
  let l:initial = 1
  if s:picker_direction ==# 'callees'
    call s:write_callee_node(l:data, '', v:true, v:true, l:lines)
  else
    let l:paths = []
    call s:collect_paths(l:data, [], l:paths)
    let l:tree = s:merge_paths(l:paths)
    for l:i in range(len(l:tree))
      call s:write_tree_node(l:tree[l:i], '', l:i == len(l:tree) - 1, l:lines)
    endfor
    let l:initial = s:first_chain_leaf_index(l:tree) + 1
  endif

  if empty(l:lines)
    echom 'callgraph: no results'
    return
  endif
  if has('nvim')
    call s:show_float(l:lines, l:initial)
  else
    call s:show_popup(l:lines, l:initial)
  endif
  echo ''
endfunction

" Jump to the location on the current line. Called from popup/float mappings.
function! callgraph#jump() abort
  let l:idx = line('.') - 1

  " Close the float / popup first.
  if has('nvim')
    let l:float_win = win_getid()
    close
  endif

  if l:idx >= 0 && l:idx < len(s:locations)
    let l:loc = s:locations[l:idx]
    execute 'edit ' . fnameescape(l:loc.file)
    execute l:loc.line
    normal! zz
  endif
endfunction

" ─── Top-down tree builder ────────────────────────────────────────────────────

" Walk the callers tree (target -> callers) and collect every path reversed
" to [root_caller, ..., target] order.
function! s:collect_paths(node, suffix, out) abort
  let l:entry = {'name': a:node.name, 'pkg': get(a:node, 'pkg', ''), 'file': a:node.file, 'line': a:node.line}
  let l:current = [l:entry] + a:suffix

  if !has_key(a:node, 'callers') || empty(a:node.callers)
    call add(a:out, l:current)
    return
  endif

  for l:caller in a:node.callers
    call s:collect_paths(l:caller, l:current, a:out)
  endfor
endfunction

" Merge a list of paths into a tree, collapsing shared prefixes.
" Returns a list of root-level tree nodes (dicts with 'entry' and 'children').
function! s:merge_paths(paths) abort
  let l:root = []
  for l:path in a:paths
    call s:insert_path(l:root, l:path)
  endfor
  return l:root
endfunction

function! s:insert_path(children, path) abort
  if empty(a:path)
    return
  endif

  let l:head = a:path[0]
  let l:rest = a:path[1:]

  for l:child in a:children
    if l:child.entry.name ==# l:head.name
          \ && l:child.entry.file ==# l:head.file
          \ && l:child.entry.line == l:head.line
      call s:insert_path(l:child.children, l:rest)
      return
    endif
  endfor

  let l:node = {'entry': l:head, 'children': []}
  call add(a:children, l:node)
  call s:insert_path(l:node.children, l:rest)
endfunction

" Recursively write display lines and populate s:locations.
function! s:write_tree_node(node, prefix, is_last, lines) abort
  let l:pkg = get(a:node.entry, 'pkg', '')
  let l:label = printf('%s (%s)', a:node.entry.name, l:pkg)

  if a:prefix ==# ''
    call add(a:lines, l:label)
  else
    call add(a:lines, a:prefix . '|__ ' . l:label)
  endif
  call add(s:locations, {'file': a:node.entry.file, 'line': a:node.entry.line})

  if a:prefix ==# ''
    let l:child_prefix = '  '
  elseif a:is_last
    let l:child_prefix = a:prefix . '    '
  else
    let l:child_prefix = a:prefix . '|   '
  endif

  for l:i in range(len(a:node.children))
    call s:write_tree_node(a:node.children[l:i], l:child_prefix,
          \ l:i == len(a:node.children) - 1, a:lines)
  endfor
endfunction

" Kind codes from internal/classify.Kind.
let s:kind_icons = {1: '🌐 ', 2: '🛢️ ', 3: '🧵 '}

" Render an outgoing-call node (and its Callees) top-down.
function! s:write_callee_node(node, prefix, is_last, is_root, lines) abort
  let l:kind = get(a:node, 'kind', 0)
  let l:icon = get(s:kind_icons, l:kind, '')
  let l:detail = get(a:node, 'detail', '')
  if l:kind == 1 && l:detail !=# ''
    let l:label = l:icon . toupper(a:node.name) . ' ' . l:detail
  elseif l:kind == 2
    let l:label = l:icon . a:node.name
    if l:detail !=# ''
      let l:label .= ' → ' . l:detail
    endif
  else
    let l:label = printf('%s%s (%s)', l:icon, a:node.name, get(a:node, 'pkg', ''))
    if l:detail !=# ''
      let l:label .= ' → ' . l:detail
    endif
  endif

  if a:is_root
    call add(a:lines, l:label)
  else
    call add(a:lines, a:prefix . '|__ ' . l:label)
  endif
  call add(s:locations, {'file': a:node.file, 'line': a:node.line})

  if a:is_root
    let l:child_prefix = '  '
  elseif a:is_last
    let l:child_prefix = a:prefix . '    '
  else
    let l:child_prefix = a:prefix . '|   '
  endif

  let l:children = get(a:node, 'callees', [])
  for l:i in range(len(l:children))
    call s:write_callee_node(l:children[l:i], l:child_prefix,
          \ l:i == len(l:children) - 1, v:false, a:lines)
  endfor
endfunction

" Count nodes along the first-child path of the first root tree.
" Returns the 0-based line index of that leaf.
function! s:first_chain_leaf_index(tree) abort
  if empty(a:tree)
    return 0
  endif
  let l:idx = 0
  let l:node = a:tree[0]
  while !empty(l:node.children)
    let l:idx += 1
    let l:node = l:node.children[0]
  endwhile
  return l:idx
endfunction

" ─── Vim 8.2+ popup ─────────────────────────────────────────────────────────

function! s:show_popup(lines, initial) abort
  let l:winid = popup_atcursor(a:lines, #{
        \ border:     [],
        \ padding:    [0, 1, 0, 1],
        \ maxwidth:   80,
        \ maxheight:  20,
        \ filter:     function('s:popup_filter'),
        \ mapping:    0,
        \ cursorline: 1,
        \ title:      ' Call Graph ',
        \ })
  call win_execute(l:winid, 'call cursor(' . a:initial . ', 1)')
endfunction

function! s:popup_filter(winid, key) abort
  if a:key ==# 'q' || a:key ==# "\<Esc>"
    call popup_close(a:winid)
    return 1
  endif

  if a:key ==# 'j' || a:key ==# "\<Down>"
    call win_execute(a:winid, 'normal! j')
    return 1
  endif

  if a:key ==# 'k' || a:key ==# "\<Up>"
    call win_execute(a:winid, 'normal! k')
    return 1
  endif

  if a:key ==# "\<CR>"
    let l:pos = getcurpos(a:winid)
    let l:idx = l:pos[1] - 1
    call popup_close(a:winid)
    if l:idx >= 0 && l:idx < len(s:locations)
      let l:loc = s:locations[l:idx]
      execute 'edit ' . fnameescape(l:loc.file)
      execute l:loc.line
      normal! zz
    endif
    return 1
  endif

  return 0
endfunction

" ─── Neovim floating window ─────────────────────────────────────────────────

function! s:show_float(lines, initial) abort
  let l:buf = nvim_create_buf(v:false, v:true)
  call nvim_buf_set_lines(l:buf, 0, -1, v:false, a:lines)

  call setbufvar(l:buf, '&modifiable', 0)
  call setbufvar(l:buf, '&bufhidden', 'wipe')
  call setbufvar(l:buf, '&buftype', 'nofile')

  let l:width  = min([max(map(copy(a:lines), 'strwidth(v:val)')) + 4, 80])
  let l:height = min([len(a:lines), 20])

  let l:opts = {
        \ 'relative':  'cursor',
        \ 'row':       1,
        \ 'col':       0,
        \ 'width':     l:width,
        \ 'height':    l:height,
        \ 'style':     'minimal',
        \ 'border':    'rounded',
        \ }

  " title requires Neovim >= 0.9
  if has('nvim-0.9')
    let l:opts.title     = ' Call Graph '
    let l:opts.title_pos = 'center'
  endif

  let l:win = nvim_open_win(l:buf, v:true, l:opts)

  call nvim_win_set_cursor(l:win, [a:initial, 0])
  setlocal cursorline

  " Buffer-local mappings
  nnoremap <buffer> <silent> q     <Cmd>close<CR>
  nnoremap <buffer> <silent> <Esc> <Cmd>close<CR>
  nnoremap <buffer> <silent> <CR>  <Cmd>call callgraph#jump()<CR>
endfunction
