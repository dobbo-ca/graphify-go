`include "defs.svh"

function int add(int a, int b);
  return a + b;
endfunction

function int compute(int x);
  return add(x, 1);
endfunction

class Counter;
  int total;
  function void step();
    total = compute(2);
  endfunction
endclass

module alu;
endmodule
