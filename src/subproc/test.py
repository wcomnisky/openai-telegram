from operator import mul as m, sub as s
print(globals(), vars())

def factorial(n):
    if n == 0:
        return 1
    else:
        return m(n, factorial(n-1))

x = 10
print(x)
factorial(x)