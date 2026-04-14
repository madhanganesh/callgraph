" autoload/callgraph.vim — Call graph popup for Go, Python, and Rust files.
"
" Requires: callgraph binary in $PATH (or set g:callgraph_binary).
" Works in Vim 8.2+ (popup) and Neovim 0.9+ (floating window).

" Parallel array: s:locations[i] corresponds to display line i+1.
" Each entry is {'file': ..., 'line': ...}.
let s:locations = []

" ─── Public ──────────────────────────────────────────────────────────────────

let s:supported_filetypes = {'go': 1, 'python': 1, 'rust': 1}

function! callgraph#show() abort
  if !has_key(s:supported_filetypes, &filetype)
    echohl WarningMsg | echom 'callgraph: unsupported filetype (need go, python, or rust)' | echohl None
    return
  endif

  let l:bin  = get(g:, 'callgraph_binary', 'callgraph')
  let l:file = expand('%:p')
  let l:line = line('.')
  let l:col  = col('.')

  echom 'callgraph: loading...'

  let l:cmd = printf('%s --file=%s --line=%d --col=%d --format=json',
        \ shellescape(l:bin),
        \ shellescape(l:file),
        \ l:line, l:col)

  let l:raw = system(l:cmd)
  if v:shell_error != 0
    echohl ErrorMsg
    echom 'callgraph: ' . substitute(l:raw, '\n', ' ', 'g')
    echohl None
    return
  endif

  let l:data = json_decode(l:raw)

  " Collect all root-to-target paths (reversed from the callers tree).
  let l:paths = []
  call s:collect_paths(l:data, [], l:paths)

  " Merge paths into a display tree and build lines + locations.
  let s:locations = []
  let l:lines = []
  let l:tree = s:merge_paths(l:paths)
  for l:i in range(len(l:tree))
    call s:write_tree_node(l:tree[l:i], '', l:i == len(l:tree) - 1, l:lines)
  endfor

  if empty(l:lines)
    echom 'callgraph: no callers found'
    return
  endif

  " Default selection: leaf of the first call chain (bottom of first branch).
  let l:initial = s:first_chain_leaf_index(l:tree) + 1

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
