import sys
import code
import requests
import requests
from importlib import import_module
from restricted import safe_globals


ETX = chr(3)  # End of text, Ctrl-C
EOT = chr(4)  # End of transmission, Ctrl-D

safe_modules = {
    'math', 'random', 'time', 'datetime', 'json', 're', 
    'itertools', 'functools', 'operator', 'collections', 
    'heapq', 'bisect', 'array', 'queue', 'threading', 'types',
    'typing', 'abc', 'contextlib', 'dataclasses', 'enum', 'copy',
    'requests', 'bs4', 'flask', 'opencv', 'ntlk',
    'numbers', 'pprint', 'numpy', 'scipy', 'pandas', 'matplotlib',
    'sklearn', 'statsmodels', 'torch', 'tensorflow', 'keras',
}

def safe_import(name, *args, **kwargs):
    if name in safe_modules:
        return import_module(name)
    else:
        raise ImportError(f'import {name} is not allowed')

def fetch(url, method='GET', headers=None, params=None, data=None, json=None):
    response = requests.request(method, url, headers=headers, params=params, data=data, json=json)
    response.raise_for_status() # raises exception if the status code is not 200 OK
    return response.text

def read_line():
    buf = []
    while 1:
        c = sys.stdin.read(1)
        if c == EOT:
            raise KeyboardInterrupt
        if c in (ETX, '\n'):
            break
        buf.append(c)
    return ''.join(buf), c


class PythonConsole(code.InteractiveConsole):
    name = 'Python Console'
    
    def __init__(self, namespace=None, filename="<console>"):
        super().__init__(namespace, filename)
        # self.compile = compile_restricted
        self.locals = safe_globals
        self.locals['__builtins__'].update(
            print=print, __import__=safe_import, vars=vars,
            globals=lambda: self.locals, locals=locals, 
            min=min, max=max, dict=dict, list=list, iter=iter,
            sum=sum, all=all, any=any, map=map, filter=filter,
            enumerate=enumerate, getattr=getattr, hasattr=hasattr,
            fetch=fetch, requests=requests,
        )

    def interact(self):
        self.info('Started')
        
        more = False
        while 1:
            try:
                line, end = read_line()
                self.info(f"Received: {bytes(line+end, 'ascii')}")
            except KeyboardInterrupt:
                self.resetbuffer()
                break
            
            if more and (end == ETX or line and not line[0].isspace()):
                self.push('\n')  # end an indent block

            more = self.push(line)
            
            if end == ETX:  # end of message
                if more:  # incomplete 
                    self.write("SyntaxError: unexpected EOF while parsing\n"+ETX)
                    self.resetbuffer()
                else:
                    self.info("Finished computing")
                    self.write(ETX)

        self.info('Exited')

    def write(self, data):
        sys.stdout.write(data)
        sys.stdout.flush()

    def info(self, data):
        sys.stderr.write(f"{self.name}: {data}\n")


PythonConsole().interact()
