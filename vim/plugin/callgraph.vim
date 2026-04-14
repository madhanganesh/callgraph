if exists('g:loaded_callgraph')
  finish
endif
let g:loaded_callgraph = 1

command! CallGraph call callgraph#show()

nnoremap <silent> <Plug>(callgraph) :call callgraph#show()<CR>

if !hasmapto('<Plug>(callgraph)') && maparg('<leader>cc', 'n') ==# ''
  nmap <leader>cc <Plug>(callgraph)
endif
