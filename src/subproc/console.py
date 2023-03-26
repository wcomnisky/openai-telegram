import sys
import code
import time
import requests
from importlib import import_module
from RestrictedPython import safe_globals # , compile_restricted

safe_import = {
    'math', 'random', 'time', 'datetime', 'json', 're', 
    'itertools', 'functools', 'operator', 'collections', 
    'heapq', 'bisect', 'array', 'queue', 'threading', 'types',
    'typing', 'abc', 'contextlib', 'dataclasses', 'enum', 'copy',
    'numbers', 'pprint', 'numpy', 'scipy', 'pandas', 'matplotlib',
    'sklearn', 'statsmodels', 'torch', 'tensorflow', 'keras',
}

def end_of_message():
    sys.stdout.write(chr(4))
    sys.stdout.flush()
    time.sleep(1)
    
def read_line():
    buf = []
    while 1:
        c = sys.stdin.read(1)
        if c == chr(3):  # ETX
            raise KeyboardInterrupt
        if c in (chr(4), '\n'):
            break
        buf.append(c)
    return ''.join(buf), c

class PythonConsole(code.InteractiveConsole):
    def __init__(self, namespace=None, filename="<console>"):
        super().__init__(namespace, filename)
        # self.compile = compile_restricted
        self.locals = safe_globals
        self.locals['__builtins__'].update(
            print=print, __import__=self.safe_import,
            globals=lambda: self.locals, locals=locals, vars=vars,
            min=min, max=max, dict=dict, list=list, iter=iter,
            sum=sum, all=all, any=any, map=map, filter=filter,
            enumerate=enumerate, getattr=getattr, hasattr=hasattr,
        )

    def safe_import(self, name, *args, **kwargs):
        if name in safe_import:
            return import_module(name)
        else:
            raise ImportError(f'import {name} is not allowed')

    def interact(self):
        self.info('Started ' + self.__class__.__name__)
        more = False
        while 1:
            try:
                line, end = read_line()
                if not line: continue
                self.info(line)
                if more and (end == chr(4) or not line[0].isspace()):
                    self.push('\n')  # end a block
                more = self.push(line)
                if end == chr(4):  # EOT
                    if more:  # incomplete 
                        sys.stdout.write("\n... ")
                    end_of_message()
            except KeyboardInterrupt:
                self.resetbuffer()
                break
        self.info('Exited ' + self.__class__.__name__)
        
    def write(self, data):
        sys.stdout.write(data)

    def info(self, data):
        sys.stderr.write(data + '\n')

PythonConsole().interact()
